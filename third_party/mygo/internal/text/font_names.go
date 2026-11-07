package text

import (
	"bytes"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"math"
)

type namedFace struct {
	name, family    string
	weight, stretch int
	italic          bool
}

// Imported selections use PostScript names, so weights in the same family
// remain independently selectable alongside permanent bundled faces.
func fontNames(data []byte) []namedFace {
	loaders, err := opentype.NewLoaders(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	var out []namedFace
	for _, ld := range loaders {
		raw, err := ld.RawTable(opentype.MustNewTag("name"))
		if err != nil {
			continue
		}
		names, _, err := tables.ParseName(raw)
		if err != nil {
			continue
		}
		name := names.Name(6)
		if name == "" {
			continue
		}
		desc, _ := font.Describe(ld, nil)
		stretch := 0
		distance := float32(math.MaxFloat32)
		for i, value := range []float32{.5, .625, .75, .875, 1, 1.125, 1.25, 1.5, 2} {
			d := float32(math.Abs(float64(float32(desc.Aspect.Stretch) - value)))
			if d < distance {
				stretch = i
				distance = d
			}
		}
		out = append(out, namedFace{name, desc.Family, int(desc.Aspect.Weight), stretch, desc.Aspect.Style == font.StyleItalic})
	}
	return out
}
