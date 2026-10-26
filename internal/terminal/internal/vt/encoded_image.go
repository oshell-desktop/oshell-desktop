package vt

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "oshell-desktop/internal/tiff"
)

// Swift's f=100 path uses ImageIO rather than requiring a PNG signature. Keep
// raw pixel coordinates here: the native placement applies display orientation
// separately, while explicit crops and the protocol cursor use the raw image.
func decodeKittyEncodedImage(data []byte) image.Image {
	config, format, valid := kittyEncodedImageConfig(data)
	if !valid {
		return nil
	}
	return decodeKittyImagePixels(data, config, format)
}

// A Kitty query reads only image metadata. In particular, a successful query
// does not promise that the pixel stream can be decoded by a later placement.
func kittyEncodedImageConfig(data []byte) (image.Config, string, bool) {
	if len(data) == 0 || len(data) > TerminalImageLimit {
		return image.Config{}, "", false
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !kittyEncodedImageSize(config.Width, config.Height) {
		return image.Config{}, "", false
	}
	// Only explicitly supported codecs may decode terminal output. Other
	// application imports can register additional local-file image formats.
	switch format {
	case "png", "jpeg", "gif", "bmp", "tiff", "webp":
		return config, format, true
	default:
		return image.Config{}, "", false
	}
}

func decodeKittyImagePixels(data []byte, config image.Config, format string) image.Image {
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format || decoded == nil {
		return nil
	}
	canvas := image.Rect(0, 0, config.Width, config.Height)
	bounds := decoded.Bounds()
	if bounds.Empty() || !bounds.In(canvas) {
		return nil
	}
	if bounds != canvas {
		if format != "gif" {
			return nil
		}
		// gif.Decode exposes the first frame's subrectangle. The terminal
		// raster uses the logical canvas, preserving offset and transparency.
		first := image.NewNRGBA(canvas)
		draw.Draw(first, canvas, decoded, image.Point{}, draw.Src)
		return first
	}
	return decoded
}

func kittyEncodedImageSize(width, height int) bool {
	return width > 0 && height > 0 && width <= 10000 && height <= 10000 && width <= TerminalImageLimit/4/height
}
