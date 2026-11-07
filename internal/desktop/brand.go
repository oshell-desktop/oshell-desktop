package desktop

import (
	_ "embed"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:generate go run ../../scripts/render-app-icon.go assets/icon.svg assets/icon.png
//go:embed assets/icon.png
var brandPNG []byte
var brandImage = sync.OnceValue(func() *ui.Bitmap {
	b, err := ui.DecodeBitmap(brandPNG)
	if err != nil {
		panic(err)
	}
	return b
})
