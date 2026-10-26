package desktop

import (
	"fmt"
	"github.com/egoist/mygo/transfer"
	"oshell-desktop/internal/core"
	"slices"
)

var clipboardImageFormats = []transfer.Format{transfer.PNG, "image/tiff", "image/jpeg"}

func readClipboardImageRepresentations(formats []transfer.Format, read func(transfer.Format) ([]byte, error)) ([][]byte, error) {
	var images [][]byte
	var last error
	for _, format := range clipboardImageFormats {
		if !slices.Contains(formats, format) {
			continue
		}
		data, err := read(format)
		if err != nil {
			last = err
			continue
		}
		images = append(images, data)
	}
	if len(images) > 0 {
		return images, nil
	}
	return nil, last
}
func normalizeClipboardRepresentations(images [][]byte) ([]byte, error) {
	var last error = fmt.Errorf("剪贴板没有可读取的图片")
	for _, data := range images {
		png, err := core.NormalizeClipboardImage(data)
		if err == nil {
			return png, nil
		}
		last = err
	}
	return nil, last
}
