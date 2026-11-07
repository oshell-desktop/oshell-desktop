//go:build ignore

// Export the editable brand artwork for the native packagers and application UI.
package main

import (
	"bytes"
	"image/png"
	"log"
	"os"

	"github.com/egoist/mygo/ui"
)

func main() {
	input, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	icon := ui.MustParseSVG(input)
	const size = 1024
	img := ui.Render(func(c *ui.Context) {
		theme := ui.LightTheme()
		theme.Background = ui.Transparent
		c.SetTheme(theme)
		ui.Image(c, icon).Size(size, size)
	}, size, size, 1)
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(os.Args[2], data.Bytes(), 0644); err != nil {
		log.Fatal(err)
	}
}
