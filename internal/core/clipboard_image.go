package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"oshell-desktop/internal/imageorient"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gen2brain/h265/heic"
	"github.com/pkg/sftp"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "oshell-desktop/internal/tiff"
)

const ClipboardImageInputLimit = 160 << 20
const ClipboardImageLimit = 20 << 20

// File attachments have a stricter limit than raw clipboard pixel representations.
func ReadClipboardImageFile(name string) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > ClipboardImageLimit {
		return nil, fmt.Errorf("图片文件过大或不可读取，单张最多 20 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, ClipboardImageLimit+1))
	if len(data) > ClipboardImageLimit {
		return nil, fmt.Errorf("图片文件超过 20 MiB")
	}
	return data, err
}

// NormalizeClipboardImage validates dimensions before allocating decoded pixels.
func NormalizeClipboardImage(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > ClipboardImageInputLimit {
		return nil, fmt.Errorf("图片为空或超过 160 MiB")
	}
	c, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("不支持或损坏的图片：%w", err)
	}
	if c.Width <= 0 || c.Height <= 0 || c.Width > 20000 || c.Height > 20000 || int64(c.Width)*int64(c.Height) > 40000000 {
		return nil, fmt.Errorf("图片尺寸过大，最多 4000 万像素且单边不超过 20000")
	}
	var im image.Image
	var colors clipboardColorMetadata
	if format == "heic" {
		// HEIF item transforms are separate from JPEG/TIFF EXIF orientation.
		// Bound coded frames as well as the container's advertised dimensions.
		var info heic.ColorInfo
		im, info, err = heic.DecodeColor(bytes.NewReader(data), heic.Options{AutoRotate: true, FrameSizeLimit: 40000000, Threads: 2})
		if err == nil {
			colors, err = clipboardHEICColors(info)
		}
	} else {
		colors, err = clipboardImageColors(data, format)
		if err == nil {
			im, _, err = image.Decode(bytes.NewReader(data))
		}
	}
	if err != nil {
		return nil, err
	}
	bounds := im.Bounds()
	if bounds.Empty() || bounds.Dx() > 20000 || bounds.Dy() > 20000 || int64(bounds.Dx())*int64(bounds.Dy()) > 40000000 {
		return nil, fmt.Errorf("解码后的图片尺寸超过限制")
	}
	im = imageorient.Apply(im, data)
	var out bytes.Buffer
	var pixels []byte
	if len(colors.icc) != 0 && string(colors.icc[16:20]) == "GRAY" &&
		im.ColorModel() != color.GrayModel && im.ColorModel() != color.Gray16Model {
		depth := byte(8)
		if im.ColorModel() == color.NRGBA64Model || im.ColorModel() == color.RGBA64Model {
			depth = 16
		}
		pixels, err = encodeClipboardGrayAlpha(im, depth)
	} else {
		err = png.Encode(&out, im)
		pixels = out.Bytes()
	}
	if err != nil {
		return nil, err
	}
	encoded, err := colors.insertPNG(pixels)
	if err != nil {
		return nil, err
	}
	if len(encoded) > ClipboardImageLimit {
		return nil, fmt.Errorf("转换后的 PNG 超过 20 MiB")
	}
	return encoded, nil
}

func SaveClipboardImage(dataDir string, data []byte, now time.Time) (string, error) {
	if len(data) == 0 || len(data) > ClipboardImageLimit {
		return "", fmt.Errorf("图片大小无效")
	}
	dir := filepath.Join(dataDir, "clipboard-images")
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("图片缓存目录不能是链接或普通文件")
	}
	if err := validateImageCacheDirectory(dir); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var used int64
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "oshell-clipboard-") || !strings.HasSuffix(entry.Name(), ".png") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if now.Sub(info.ModTime()) > 7*24*time.Hour && root.Remove(entry.Name()) == nil {
			continue
		}
		used += info.Size()
	}
	if used+int64(len(data)) > 200<<20 {
		return "", fmt.Errorf("图片缓存超过 200 MiB，请清理旧图片后再试")
	}
	name := "oshell-clipboard-" + NewID() + ".png"
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	err = errors.Join(err, f.Close())
	if err != nil {
		_ = root.Remove(name)
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// UploadClipboardImage uses a private, unique directory and never overwrites a file.
// The caller owns the dedicated SFTP channel and closes it on cancellation.
func UploadClipboardImage(ctx context.Context, client *sftp.Client, base string, data []byte, progress ...func(int64, int64)) (result string, err error) {
	if len(data) == 0 || len(data) > ClipboardImageLimit {
		return "", fmt.Errorf("图片大小无效")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if base == "" || strings.IndexFunc(base, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("请输入有效的远端目录")
	}
	base, err = client.RealPath(base)
	if err != nil {
		return "", err
	}
	info, err := client.Stat(base)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("远端目标不是目录")
	}
	dir := path.Join(base, "oshell-clipboard-"+NewID())
	if err = client.Mkdir(dir); err != nil {
		return "", err
	}
	file := path.Join(dir, "image.png")
	defer func() {
		if err != nil {
			_ = client.Remove(file)
			_ = client.RemoveDirectory(dir)
		}
	}()
	if err = client.Chmod(dir, 0700); err != nil {
		return "", err
	}
	info, err = client.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("服务器未按要求创建私有图片目录，已停止上传")
	}
	f, err := client.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return "", err
	}
	if err = f.Chmod(0600); err == nil {
		var written int64
		for offset := 0; offset < len(data) && err == nil; {
			if err = ctx.Err(); err != nil {
				break
			}
			var n int
			n, err = f.Write(data[offset:min(offset+32*1024, len(data))])
			offset += n
			written += int64(n)
			if len(progress) > 0 && progress[0] != nil {
				progress[0](written, int64(len(data)))
			}
			if n == 0 && err == nil {
				err = io.ErrShortWrite
			}
		}
	}
	err = errors.Join(err, f.Close(), ctx.Err())
	if err != nil {
		return "", err
	}
	return file, nil
}

func ClipboardImageShellPath(p string, windows bool) (string, error) {
	if p == "" || strings.IndexFunc(p, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("图片路径包含无效字符")
	}
	if windows {
		return "'" + strings.ReplaceAll(p, "'", "''") + "'", nil
	} // Local Windows terminals use PowerShell.
	var escaped strings.Builder
	for _, c := range p {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/_-.:+", c)) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(c)
	}
	return escaped.String(), nil
}
