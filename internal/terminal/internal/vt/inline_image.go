package vt

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"math"
	"oshell-desktop/internal/imageorient"
	"strconv"
	"strings"
	"unsafe"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	_ "golang.org/x/image/webp"
	_ "oshell-desktop/internal/tiff"
)

// Matches the private ABI 4 request prefix. The image is decoded synchronously so a
// following cursor query observes the completed insertion, even in one Write.
type inlineImageRequest struct {
	data, length  uintptr
	width, height uint32
	cols, rows    uint32
}

type sixelImageRequest struct {
	inlineImageRequest
	kind    uint32
	palette [16]uint32
}

var cbInlineImage = inlineImageCallback(false)
var cbInlineImageSixel = inlineImageCallback(true)

func inlineImageCallback(sixel bool) uintptr {
	return newCallback(func(_, allocator, request, output uintptr) uintptr {
		if request == 0 || output == 0 {
			return 0
		}
		r := (*inlineImageRequest)(mem(request))
		if r.data == 0 || r.length == 0 || r.length > 8<<20 || r.rows == 0 {
			return 0
		}
		data := unsafe.Slice((*byte)(mem(r.data)), r.length)
		var im *image.NRGBA
		if sixel && (*sixelImageRequest)(mem(request)).kind == 1 {
			im = decodeSixel(data, *r, (*sixelImageRequest)(mem(request)).palette)
		} else {
			im = decodeInlineImage(data, *r)
		}
		if im == nil {
			return 0
		}
		// Swift creates full-height row bitmaps, including a transparent tail
		// on the final stripe. Pad directly in native storage rather than
		// allocating and copying a second Go canvas.
		width := im.Bounds().Dx()
		height := rowImageCanvasHeight(width, im.Bounds().Dy(), int(r.height/r.rows))
		if height == 0 {
			return 0
		}
		n := uintptr(width * height * 4)
		pixels := call(fnAlloc, allocator, n)
		if pixels == 0 {
			return 0
		}
		dst := unsafe.Slice((*byte)(mem(pixels)), n)
		copied := copy(dst, im.Pix)
		clear(dst[copied:])
		*(*sysImage)(mem(output)) = sysImage{uint32(width), uint32(height), pixels, n}
		return 1
	})
}

func rowImageCanvasHeight(width, height, cellHeight int) int {
	if !inlineImageSize(width, height) || cellHeight <= 0 || cellHeight > TerminalImageLimit/4/width {
		return 0
	}
	padded := (1 + (height-1)/cellHeight) * cellHeight
	if !inlineImageSize(width, padded) {
		return 0
	}
	return padded
}

// A zero dimension means auto. Match SwiftTerm's strict limits and fallback;
// malformed dimensions do not reject an otherwise valid image.
func inlineDimension(value string, region, cell int) float64 {
	limit, factor := 200, float64(cell)
	switch {
	case strings.HasSuffix(value, "%"):
		value, limit, factor = strings.TrimSuffix(value, "%"), 101, float64(region)/100
	case strings.HasSuffix(value, "px"):
		value, limit, factor = strings.TrimSuffix(value, "px"), 4096, 1
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 || n >= limit {
		return 0
	}
	return float64(n) * factor
}

func decodeInlineImage(payload []byte, r inlineImageRequest) *image.NRGBA {
	if r.width == 0 || r.height == 0 || r.cols == 0 || r.rows == 0 {
		return nil
	}
	head, encoded, ok := bytes.Cut(payload, []byte{':'})
	if !ok || len(head) > 16<<10 || len(encoded) == 0 || len(encoded) > 8<<20 {
		return nil
	}
	params := make(map[string]string)
	for _, part := range strings.Split(string(head), ";") {
		key, value, found := strings.Cut(part, "=")
		if !found {
			break
		}
		params[key] = value
	}
	if params["inline"] != "1" {
		return nil // A remote File command never downloads to local storage.
	}
	data, err := base64.StdEncoding.Strict().DecodeString(string(encoded))
	if err != nil {
		return nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !inlineImageSize(config.Width, config.Height) {
		return nil
	}
	// The application can register additional codecs for local clipboard data.
	// Do not implicitly expose those codecs to untrusted terminal output.
	switch format {
	case "png", "jpeg", "gif", "bmp", "tiff", "webp":
	default:
		return nil
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil || !inlineImageSize(im.Bounds().Dx(), im.Bounds().Dy()) {
		return nil
	}
	// NSImage(data:) applies EXIF before sizing and slicing row images.
	return sizeInlineImage(imageorient.Apply(im, data), params, r)
}

func sizeInlineImage(im image.Image, params map[string]string, r inlineImageRequest) *image.NRGBA {
	sw, sh := float64(im.Bounds().Dx()), float64(im.Bounds().Dy())
	w := inlineDimension(params["width"], int(r.width), int(r.width/r.cols))
	h := inlineDimension(params["height"], int(r.height), int(r.height/r.rows))
	autoW, autoH := w == 0, h == 0
	if autoW {
		w = sw
	}
	if autoH {
		h = sh
	}
	preserve := params["preserveAspectRatio"] == "" || params["preserveAspectRatio"] == "1"
	if preserve {
		if autoH && !autoW {
			h = w * sh / sw
		} else if autoW && !autoH {
			w = h * sw / sh
		}
	}
	// NSBitmapImageRep and AppleImage truncate each stripe's width. A
	// subpixel canvas fails creation and must not advance the terminal.
	if w > TerminalImageLimit || h > TerminalImageLimit || !inlineImageSize(int(w), int(math.Ceil(h))) {
		return nil
	}
	out := image.NewNRGBA(image.Rect(0, 0, int(w), int(math.Ceil(h))))
	dw, dh := w, h
	if preserve && !autoW && !autoH {
		// Match MacTerminalView.scale, including its centered clipping when
		// both dimensions are supplied. The transparent canvas is retained.
		if sh/sw < w/h {
			dw = h * sw / sh
		} else {
			dh = w * sh / sw
		}
	}
	// Transform clips to the small output canvas before iterating. Scale would
	// allocate filters for a potentially enormous off-canvas destination when
	// an extreme source aspect ratio is centered in an explicit size.
	sx, sy := dw/sw, dh/sh
	xdraw.CatmullRom.Transform(out, f64.Aff3{sx, 0, (w-dw)/2 - sx*float64(im.Bounds().Min.X), 0, sy, (h-dh)/2 - sy*float64(im.Bounds().Min.Y)}, im, im.Bounds(), xdraw.Src, nil)
	return out
}

func inlineImageSize(w, h int) bool {
	return w > 0 && h > 0 && w <= TerminalImageLimit/4/h
}
