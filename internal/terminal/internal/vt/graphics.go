package vt

import (
	"image"
	"image/draw"
	"sort"
	"sync"
	"unsafe"

	"oshell-desktop/internal/imageorient"
)

// KittyImageLimit bounds protocol cache entries, not decoded display pixels.
const KittyImageLimit = 4 << 20

// Match SwiftTerm/Kitty's per-image safety bound independently of cache quota.
// OSC 1337 and Sixel row images do not consume the Kitty protocol cache.
const TerminalImageLimit = 400 << 20

var fnSysSet, fnAlloc, fnGraphicsGet, fnGraphicsImage, fnImageGet,
	fnPlacementNew, fnPlacementFree, fnPlacementNext, fnPlacementGet, fnPlacementInfo uintptr

func init() {
	symbols = append(symbols, []struct {
		name string
		addr *uintptr
	}{
		{"ghostty_sys_set", &fnSysSet}, {"ghostty_alloc", &fnAlloc},
		{"ghostty_kitty_graphics_get", &fnGraphicsGet},
		{"ghostty_kitty_graphics_image", &fnGraphicsImage},
		{"ghostty_kitty_graphics_image_get", &fnImageGet},
		{"ghostty_kitty_graphics_placement_iterator_new", &fnPlacementNew},
		{"ghostty_kitty_graphics_placement_iterator_free", &fnPlacementFree},
		{"ghostty_kitty_graphics_placement_next", &fnPlacementNext},
		{"ghostty_kitty_graphics_placement_get", &fnPlacementGet},
		{"ghostty_kitty_graphics_placement_render_info", &fnPlacementInfo},
	}...)
}

type sysImage struct {
	width, height uint32
	data, length  uintptr
}

// The library owns the returned allocation. Never pass it Go-managed storage.
func installGraphicsDecoder() error {
	if oshellABI >= 42 && oshellABI <= 62 {
		dimensions := newCallback(func(_, data, size, width, height uintptr) uintptr {
			if data == 0 || size == 0 || size > TerminalImageLimit || width == 0 || height == 0 {
				return 0
			}
			config, _, valid := kittyEncodedImageConfig(unsafe.Slice((*byte)(mem(data)), size))
			if !valid {
				return 0
			}
			*(*uint32)(mem(width)) = uint32(config.Width)
			*(*uint32)(mem(height)) = uint32(config.Height)
			return 1
		})
		if err := result(call(fnSysSet, 1003, dimensions)); err != nil {
			return err
		}
	}
	if oshellABI >= 41 && oshellABI <= 62 {
		metadata := newCallback(func(_, data, size uintptr) uintptr {
			if data == 0 || size == 0 || size > TerminalImageLimit {
				return 0
			}
			_, _, valid := kittyEncodedImageConfig(unsafe.Slice((*byte)(mem(data)), size))
			if valid {
				return 1
			}
			return 0
		})
		if err := result(call(fnSysSet, 1002, metadata)); err != nil {
			return err
		}
	}
	if oshellABI >= 38 && oshellABI <= 62 {
		orientation := newCallback(func(_, data, size uintptr) uintptr {
			if data == 0 || size == 0 || size > TerminalImageLimit {
				return 1
			}
			return uintptr(imageorient.Orientation(unsafe.Slice((*byte)(mem(data)), size)))
		})
		if err := result(call(fnSysSet, 1001, orientation)); err != nil {
			return err
		}
	}
	callback := newCallback(func(_, allocator, data, size, output uintptr) uintptr {
		if data == 0 || output == 0 || size == 0 || size > TerminalImageLimit {
			return 0
		}
		encoded := unsafe.Slice((*byte)(mem(data)), size)
		decoded := decodeKittyEncodedImage(encoded)
		if decoded == nil {
			return 0
		}
		width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
		n := uintptr(width * height * 4)
		pixels := call(fnAlloc, allocator, n)
		if pixels == 0 {
			return 0
		}
		im := &image.NRGBA{Pix: unsafe.Slice((*byte)(mem(pixels)), n), Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
		draw.Draw(im, im.Rect, decoded, decoded.Bounds().Min, draw.Src)
		*(*sysImage)(mem(output)) = sysImage{uint32(width), uint32(height), pixels, n}
		return 1
	})
	return result(call(fnSysSet, 1, callback))
}

// Layout matches GhosttyKittyGraphicsPlacementRenderInfo's sized C ABI.
type graphicsRenderInfo struct {
	size                            uintptr
	width, height, cols, rows       uint32
	col, row                        int32
	visible                         bool
	_                               [3]byte
	x, y, sourceWidth, sourceHeight uint32
}

// Graphic is an owned snapshot. No borrowed native pixels survive the lock
// held by the caller while taking this snapshot. Coordinates are device pixels
// relative to the terminal grid, and Source uses the returned Image coordinates.
type Graphic struct {
	// Virtual placements supply image metadata to Unicode placeholder cells.
	// Their grid dimensions and offsets are independent of ordinary rasters.
	Virtual              bool
	Columns, Rows        int
	logicalImageID       uint32
	paintOrder           uint64
	lazyImage            *graphicPixels
	Image                *image.NRGBA
	Generation           uint64
	Orientation          uint8
	ImageID, PlacementID uint32
	X, Y, Width, Height  int
	// Nonzero exact dimensions retain the aspect-ratio transform before
	// truncating the bitmap width or rounding up its final partial row.
	ExactWidth, ExactHeight float64
	Source                  image.Rectangle
	Z                       int32
	AnchorRow               int
	LineMode                LineMode
	// A raster stripe shares the full source bitmap and destination transform,
	// but paints only its attached row canvas. Coordinates use grid pixels,
	// before line-mode and UI-scale transforms. Explicit empty clips paint none.
	Clipped    bool
	RasterClip image.Rectangle
}

// LogicalImageID is the application-visible identity, not a retained raster ID.
func (g Graphic) LogicalImageID() uint32 { return g.logicalImageID }

type graphicCacheKey struct {
	generation  uint64
	orientation uint8
}

// Graphics recomputes geometry even when image bytes have not changed (resize
// and scroll do not bump image generations). Pixel copies are reused by their
// process-wide generation, never just image ID or dimensions.
func (t *Terminal) Graphics(cellWidth, cellHeight int) []Graphic {
	return t.graphics(cellWidth, cellHeight, false)
}

// RenderGraphics also snapshots virtual placements for the text renderer.
// They never draw at the protocol cursor; only matching placeholder cells do.
func (t *Terminal) RenderGraphics(cellWidth, cellHeight int) []Graphic {
	return t.graphics(cellWidth, cellHeight, true)
}

func (t *Terminal) graphics(cellWidth, cellHeight int, includeVirtual bool) []Graphic {
	var storage, iterator uintptr
	if t.h == 0 || t.get(30, unsafe.Pointer(&storage)) != nil || storage == 0 {
		t.graphicsCache = nil
		return nil
	}
	var generation uint64
	if !ok(call(fnGraphicsGet, storage, 2, uintptr(unsafe.Pointer(&generation)))) || generation == 0 {
		t.graphicsCache = nil
		return nil
	}
	if !ok(call(fnPlacementNew, 0, uintptr(unsafe.Pointer(&iterator)))) {
		return nil
	}
	defer call(fnPlacementFree, iterator)
	if !ok(call(fnGraphicsGet, storage, 1, uintptr(unsafe.Pointer(&iterator)))) {
		return nil
	}
	next := make(map[graphicCacheKey]*graphicPixels)
	viewport := t.Scrollbar().Offset
	var graphics []Graphic
	for cbool(call(fnPlacementNext, iterator)) {
		var id, placement, offsetX, offsetY uint32
		var z int32
		if !ok(call(fnPlacementGet, iterator, 1, uintptr(unsafe.Pointer(&id)))) {
			continue
		}
		native := call(fnGraphicsImage, storage, uintptr(id))
		if native == 0 {
			continue
		}
		var info graphicsRenderInfo
		info.size = unsafe.Sizeof(info)
		var virtual bool
		call(fnPlacementGet, iterator, 3, uintptr(unsafe.Pointer(&virtual)))
		if virtual && !includeVirtual {
			continue
		}
		if virtual && oshellABI >= 46 && oshellABI <= 62 {
			var cache uintptr
			var logicalID uint32
			if t.get(1004, unsafe.Pointer(&cache)) != nil || cache == 0 || !ok(call(fnImageGet, native, 1001, uintptr(unsafe.Pointer(&logicalID)))) {
				continue
			}
			native = call(fnGraphicsImage, cache, uintptr(logicalID))
			if native == 0 {
				continue
			}
		}
		if !virtual && (!ok(call(fnPlacementInfo, iterator, native, t.h, uintptr(unsafe.Pointer(&info)))) || !info.visible || info.width == 0 || info.height == 0 || info.sourceWidth == 0 || info.sourceHeight == 0) {
			continue
		}
		var generation uint64
		if !ok(call(fnImageGet, native, 9, uintptr(unsafe.Pointer(&generation)))) {
			continue
		}
		orientation := uint8(1)
		if oshellABI >= 38 && oshellABI <= 62 {
			call(fnPlacementGet, iterator, 1002, uintptr(unsafe.Pointer(&orientation)))
		}
		if virtual {
			// Isolate encoded/oriented placeholder pixels from ordinary raw
			// crop snapshots of the same generation.
			orientation = 0
		}
		key := graphicCacheKey{generation, orientation}
		pixels := next[key]
		if pixels == nil {
			pixels = t.graphicsCache[key]
		}
		if pixels == nil {
			pixels = snapshotGraphicPixels(native, orientation, virtual)
		}
		if pixels == nil {
			continue
		}
		next[key] = pixels
		call(fnPlacementGet, iterator, 2, uintptr(unsafe.Pointer(&placement)))
		call(fnPlacementGet, iterator, 4, uintptr(unsafe.Pointer(&offsetX)))
		call(fnPlacementGet, iterator, 5, uintptr(unsafe.Pointer(&offsetY)))
		call(fnPlacementGet, iterator, 12, uintptr(unsafe.Pointer(&z)))
		logicalID := id
		var paintOrder uint64
		if oshellABI >= 8 && oshellABI <= 62 {
			call(fnImageGet, native, 1001, uintptr(unsafe.Pointer(&logicalID)))
			call(fnPlacementGet, iterator, 1001, uintptr(unsafe.Pointer(&paintOrder)))
		}
		if virtual {
			var cols, rows uint32
			call(fnPlacementGet, iterator, 10, uintptr(unsafe.Pointer(&cols)))
			call(fnPlacementGet, iterator, 11, uintptr(unsafe.Pointer(&rows)))
			graphics = append(graphics, Graphic{Virtual: true, Columns: int(cols), Rows: int(rows),
				logicalImageID: logicalID, paintOrder: paintOrder, Image: pixels.im, lazyImage: pixels.lazy(), Generation: generation, Orientation: orientation,
				ImageID: id, PlacementID: placement, X: int(offsetX), Y: int(offsetY), Source: pixels.bounds, Z: z})
			continue
		}
		ref, found := t.screenCell(0, int(viewport)+int(info.row))
		g := Graphic{logicalImageID: logicalID, paintOrder: paintOrder, Image: pixels.im, Generation: generation, Orientation: orientation, ImageID: id, PlacementID: placement,
			AnchorRow: int(info.row), LineMode: lineModeRef(ref, found),
			X: int(info.col)*cellWidth + int(offsetX), Y: int(info.row)*cellHeight + int(offsetY), Width: int(info.width), Height: int(info.height),
			Source: image.Rect(int(info.x), int(info.y), int(info.x)+int(info.sourceWidth), int(info.y)+int(info.sourceHeight)), Z: z}
		if oshellABI >= 40 && oshellABI <= 62 {
			var exact struct{ width, height float64 }
			if ok(call(fnPlacementGet, iterator, 1004, uintptr(unsafe.Pointer(&exact)))) &&
				(exact.width != float64(g.Width) || exact.height != float64(g.Height)) {
				g.ExactWidth, g.ExactHeight = exact.width, exact.height
			}
		}
		if oshellABI >= 39 && oshellABI <= 62 {
			var stripe struct{ sourceY, height uint32 }
			if ok(call(fnPlacementGet, iterator, 1003, uintptr(unsafe.Pointer(&stripe)))) {
				g.Clipped = true
				g.RasterClip = image.Rect(g.X, g.Y, g.X+g.Width, g.Y+int(stripe.height))
				g.Y -= int(stripe.sourceY)
			}
		}
		graphics = appendRasterGraphic(graphics, g)
	}
	for i := range graphics {
		normalizeRasterClip(&graphics[i])
	}
	t.graphicsCache = next
	sort.SliceStable(graphics, func(i, j int) bool {
		a, b := graphics[i], graphics[j]
		if a.Z != b.Z {
			return a.Z < b.Z
		}
		if a.logicalImageID != b.logicalImageID {
			return a.logicalImageID < b.logicalImageID
		}
		if a.paintOrder != b.paintOrder {
			return a.paintOrder < b.paintOrder
		}
		return a.PlacementID < b.PlacementID
	})
	return graphics
}

// Adjacent stripes can share one draw only while their original raster
// transforms still coincide. Row edits, font changes and double-height rows
// must retain independent clips, including when their canvases overlap.
func appendRasterGraphic(dst []Graphic, g Graphic) []Graphic {
	if g.Clipped && len(dst) > 0 && g.LineMode != LineDoubleTop && g.LineMode != LineDoubleBottom {
		p := &dst[len(dst)-1]
		if p.Clipped && p.ImageID == g.ImageID && p.paintOrder == g.paintOrder && p.Source == g.Source &&
			p.LineMode == g.LineMode && p.X == g.X && p.Y == g.Y && p.Width == g.Width && p.Height == g.Height &&
			p.ExactWidth == g.ExactWidth && p.ExactHeight == g.ExactHeight &&
			p.RasterClip.Min.X == g.RasterClip.Min.X && p.RasterClip.Max.X == g.RasterClip.Max.X && p.RasterClip.Max.Y == g.RasterClip.Min.Y {
			p.RasterClip.Max.Y = g.RasterClip.Max.Y
			return dst
		}
	}
	return append(dst, g)
}

// Unscaled source crops avoid oversized draws after history pruning. Scaled
// stripes retain the full texture: cropping it first would change interpolation
// at the stripe boundary and introduce seams between otherwise adjacent rows.
func normalizeRasterClip(g *Graphic) {
	if !g.Clipped || g.Width <= 0 || g.Height <= 0 || g.ExactWidth != 0 || g.ExactHeight != 0 {
		return
	}
	full := image.Rect(g.X, g.Y, g.X+g.Width, g.Y+g.Height)
	clip := g.RasterClip.Intersect(full)
	if clip == full {
		g.Clipped = false
		return
	}
	if clip.Empty() || g.Height != g.Source.Dy() || clip.Min.X != full.Min.X || clip.Max.X != full.Max.X {
		return
	}
	top := int64(clip.Min.Y-g.Y) * int64(g.Source.Dy())
	bottom := int64(clip.Max.Y-g.Y) * int64(g.Source.Dy())
	if top%int64(g.Height) != 0 || bottom%int64(g.Height) != 0 {
		return
	}
	g.Source.Max.Y = g.Source.Min.Y + int(bottom/int64(g.Height))
	g.Source.Min.Y += int(top / int64(g.Height))
	g.Y, g.Height = clip.Min.Y, clip.Dy()
	g.Clipped = false
}

// Encoded virtual images stay compressed until a visible placeholder needs
// them. Owned bytes may cross the terminal lock; native pointers never do.
// Keep the immutable ordinary image separate from the lazily decoded result
// so new snapshots do not race the UI thread's first decode.
type graphicPixels struct {
	im      *image.NRGBA
	bounds  image.Rectangle
	encoded []byte
	once    sync.Once
	decoded *image.NRGBA
}

func (p *graphicPixels) lazy() *graphicPixels {
	if p.im == nil {
		return p
	}
	return nil
}

// ResolveImage lazily decodes a virtual image, once per owned generation.
// It runs after the terminal lock has been released, only for visible cells.
func (g *Graphic) ResolveImage() *image.NRGBA {
	if g.lazyImage != nil {
		p := g.lazyImage
		p.once.Do(func() {
			decoded := decodeKittyEncodedImage(p.encoded)
			if decoded != nil {
				oriented := imageorient.WithOrientation(decoded, int(imageorient.Orientation(p.encoded)))
				p.decoded = image.NewNRGBA(oriented.Bounds())
				draw.Draw(p.decoded, p.decoded.Rect, oriented, oriented.Bounds().Min, draw.Src)
			}
			p.encoded = nil
		})
		g.Image = p.decoded
	}
	return g.Image
}

func snapshotGraphicPixels(native uintptr, orientation uint8, virtual bool) *graphicPixels {
	var format int32
	if virtual && ok(call(fnImageGet, native, 5, uintptr(unsafe.Pointer(&format)))) && format == 2 {
		var data, size uintptr
		if !ok(call(fnImageGet, native, 7, uintptr(unsafe.Pointer(&data)))) ||
			!ok(call(fnImageGet, native, 8, uintptr(unsafe.Pointer(&size)))) || data == 0 || size == 0 || size > TerminalImageLimit {
			return nil
		}
		encoded := append([]byte(nil), unsafe.Slice((*byte)(mem(data)), size)...)
		config, _, valid := kittyEncodedImageConfig(encoded)
		if !valid {
			return nil
		}
		w, h := config.Width, config.Height
		if imageorient.Orientation(encoded) >= 5 {
			w, h = h, w
		}
		return &graphicPixels{bounds: image.Rect(0, 0, w, h), encoded: encoded}
	}
	if im := copyGraphicPixels(native, orientation); im != nil {
		return &graphicPixels{im: im, bounds: im.Rect}
	}
	return nil
}

func copyGraphicPixels(native uintptr, orientation uint8) *image.NRGBA {
	var width, height uint32
	var format int32
	var data, size uintptr
	if !ok(call(fnImageGet, native, 3, uintptr(unsafe.Pointer(&width)))) ||
		!ok(call(fnImageGet, native, 4, uintptr(unsafe.Pointer(&height)))) ||
		!ok(call(fnImageGet, native, 5, uintptr(unsafe.Pointer(&format)))) ||
		!ok(call(fnImageGet, native, 7, uintptr(unsafe.Pointer(&data)))) ||
		!ok(call(fnImageGet, native, 8, uintptr(unsafe.Pointer(&size)))) || data == 0 || size > TerminalImageLimit || width == 0 || height == 0 {
		return nil
	}
	bpp := 0
	switch format {
	case 0:
		bpp = 3
	case 1:
		bpp = 4
	case 3:
		bpp = 2
	case 4:
		bpp = 1
	default:
		return nil
	}
	if width > TerminalImageLimit/4/height || uint64(width)*uint64(height)*uint64(bpp) != uint64(size) {
		return nil
	}
	input := unsafe.Slice((*byte)(mem(data)), size)
	if bpp == 4 && orientation >= 2 && orientation <= 8 {
		// Borrow native pixels only while this locked snapshot is being made.
		// Transform directly into the owned result, without a second full copy.
		raw := &image.NRGBA{Pix: input, Stride: int(width) * 4, Rect: image.Rect(0, 0, int(width), int(height))}
		view := imageorient.WithOrientation(raw, int(orientation))
		im := image.NewNRGBA(view.Bounds())
		draw.Draw(im, im.Rect, view, view.Bounds().Min, draw.Src)
		return im
	}
	im := image.NewNRGBA(image.Rect(0, 0, int(width), int(height)))
	if bpp == 4 {
		copy(im.Pix, input)
		return im
	}
	for i, j := 0, 0; i < len(input); i, j = i+bpp, j+4 {
		if bpp == 3 {
			copy(im.Pix[j:j+3], input[i:i+3])
		} else {
			im.Pix[j], im.Pix[j+1], im.Pix[j+2] = input[i], input[i], input[i]
		}
		im.Pix[j+3] = 255
		if bpp == 2 {
			im.Pix[j+3] = input[i+1]
		}
	}
	return im
}
