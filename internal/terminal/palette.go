package terminal

// Adapted from SwiftTerm Colors.swift (OShell 0.2.92).
// Copyright (c) 2019-2026 Miguel de Icaza and SwiftTerm contributors.
// Distributed under the MIT license; see resources/licenses/SwiftTerm-LICENSE.

import (
	"math"
	"oshell-desktop/internal/terminal/internal/vt"
)

type labColor struct{ l, a, b float64 }

func fillLabPalette(p *[256]vt.RGB, background, foreground vt.RGB) {
	var anchors [8]labColor
	for i := range anchors {
		anchors[i] = rgbToLab(p[i])
	}
	anchors[0], anchors[7] = rgbToLab(background), rgbToLab(foreground)
	if anchors[7].l < anchors[0].l {
		anchors[0], anchors[7] = anchors[7], anchors[0]
	}
	for r := range 6 {
		tr := float64(r) / 5
		c0, c1 := lerpLab(tr, anchors[0], anchors[1]), lerpLab(tr, anchors[2], anchors[3])
		c2, c3 := lerpLab(tr, anchors[4], anchors[5]), lerpLab(tr, anchors[6], anchors[7])
		for g := range 6 {
			tg := float64(g) / 5
			c4, c5 := lerpLab(tg, c0, c1), lerpLab(tg, c2, c3)
			for b := range 6 {
				p[16+r*36+g*6+b] = lerpLab(float64(b)/5, c4, c5).rgb()
			}
		}
	}
	for i := range 24 {
		p[232+i] = lerpLab(float64(i+1)/25, anchors[0], anchors[7]).rgb()
	}
}

func rgbToLab(c vt.RGB) labColor {
	linear := func(v uint8) float64 {
		s := float64(v) / 255
		if s > 0.04045 {
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return s / 12.92
	}
	r, g, b := linear(c.R), linear(c.G), linear(c.B)
	f := func(v float64) float64 {
		if v > 0.008856 {
			return math.Pow(v, 1.0/3)
		}
		return 7.787*v + 16.0/116
	}
	x := f((r*0.4124564 + g*0.3575761 + b*0.1804375) / 0.95047)
	y := f(r*0.2126729 + g*0.7151522 + b*0.0721750)
	z := f((r*0.0193339 + g*0.1191920 + b*0.9503041) / 1.08883)
	return labColor{116*y - 16, 500 * (x - y), 200 * (y - z)}
}

func (c labColor) rgb() vt.RGB {
	y := (c.l + 16) / 116
	x, z := c.a/500+y, y-c.b/200
	f := func(v float64) float64 {
		if cube := v * v * v; cube > 0.008856 {
			return cube
		}
		return (v - 16.0/116) / 7.787
	}
	xf, yf, zf := f(x)*0.95047, f(y), f(z)*1.08883
	quantize := func(v float64) uint8 {
		if v > 0.0031308 {
			v = 1.055*math.Pow(v, 1.0/2.4) - 0.055
		} else {
			v *= 12.92
		}
		return uint8(max(0, min(1, v))*255 + 0.5)
	}
	return vt.RGB{
		R: quantize(xf*3.2404542 - yf*1.5371385 - zf*0.4985314),
		G: quantize(-xf*0.9692660 + yf*1.8760108 + zf*0.0415560),
		B: quantize(xf*0.0556434 - yf*0.2040259 + zf*1.0572252),
	}
}

func lerpLab(t float64, from, to labColor) labColor {
	return labColor{from.l + t*(to.l-from.l), from.a + t*(to.a-from.a), from.b + t*(to.b-from.b)}
}
