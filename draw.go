package main

import (
	"bytes"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"odyssey9/machine"
)

// canvas draws in logical coordinates onto a render target that is s times
// larger, so the window stays sharp at any size and pixel density.
type canvas struct {
	dst *ebiten.Image
	s   float32
}

func (c canvas) rect(x, y, w, h float32, col color.Color) {
	vector.DrawFilledRect(c.dst, x*c.s, y*c.s, w*c.s, h*c.s, col, false)
}

func (c canvas) line(x0, y0, x1, y1, width float32, col color.Color) {
	vector.StrokeLine(c.dst, x0*c.s, y0*c.s, x1*c.s, y1*c.s, width*c.s, col, true)
}

func (c canvas) circle(cx, cy, r float32, col color.Color) {
	vector.DrawFilledCircle(c.dst, cx*c.s, cy*c.s, r*c.s, col, true)
}

// rrect fills a rounded rectangle.
func (c canvas) rrect(x, y, w, h, r float32, col color.Color) {
	r = float32(math.Min(float64(r), float64(math.Min(float64(w), float64(h))/2)))
	c.rect(x+r, y, w-2*r, h, col)
	c.rect(x, y+r, w, h-2*r, col)
	c.circle(x+r, y+r, r, col)
	c.circle(x+w-r, y+r, r, col)
	c.circle(x+r, y+h-r, r, col)
	c.circle(x+w-r, y+h-r, r, col)
}

// rframe draws a rounded outline of thickness t over a solid background.
func (c canvas) rframe(x, y, w, h, r, t float32, col, bg color.Color) {
	c.rrect(x, y, w, h, r, col)
	c.rrect(x+t, y+t, w-2*t, h-2*t, r-t, bg)
}

type fontKind int

const (
	fontRegular fontKind = iota
	fontBold
	fontMono
)

var fontSources = func() map[fontKind]*text.GoTextFaceSource {
	load := func(b []byte) *text.GoTextFaceSource {
		s, err := text.NewGoTextFaceSource(bytes.NewReader(b))
		if err != nil {
			panic(err)
		}
		return s
	}
	return map[fontKind]*text.GoTextFaceSource{
		fontRegular: load(goregular.TTF),
		fontBold:    load(gobold.TTF),
		fontMono:    load(gomono.TTF),
	}
}()

func (c canvas) face(kind fontKind, size float32) *text.GoTextFace {
	return &text.GoTextFace{Source: fontSources[kind], Size: float64(size * c.s)}
}

// text draws a string; align is -1 (left), 0 (centre) or 1 (right) and y is
// the vertical centre of the line.
func (c canvas) text(s string, kind fontKind, size, x, y float32, col color.Color, align int) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x*c.s), float64(y*c.s))
	op.ColorScale.ScaleWithColor(col)
	op.PrimaryAlign = [...]text.Align{text.AlignStart, text.AlignCenter, text.AlignEnd}[align+1]
	op.SecondaryAlign = text.AlignCenter
	text.Draw(c.dst, s, c.face(kind, size), op)
}

func (c canvas) textWidth(s string, kind fontKind, size float32) float32 {
	w, _ := text.Measure(s, c.face(kind, size), 0)
	return float32(w) / c.s
}

// glyphImages holds one 8x7 white image per Odyssey2 character. It is
// built on first use because images need the game loop to be running.
var glyphImages [64]*ebiten.Image

func initGlyphs() {
	for code := range glyphImages {
		pix := make([]byte, 4*machine.GlyphWidth*machine.GlyphHeight)
		for y, bits := range machine.Glyph(byte(code)) {
			for x := range machine.GlyphWidth {
				if bits&(0x80>>x) != 0 {
					copy(pix[4*(y*machine.GlyphWidth+x):], []byte{0xFF, 0xFF, 0xFF, 0xFF})
				}
			}
		}
		img := ebiten.NewImage(machine.GlyphWidth, machine.GlyphHeight)
		img.WritePixels(pix)
		glyphImages[code] = img
	}
}

// glyph draws an Odyssey2 character with each pixel px by py logical units.
func (c canvas) glyph(code byte, x, y, px, py float32, col color.Color) {
	if int(code) >= len(glyphImages) {
		return
	}
	if glyphImages[0] == nil {
		initGlyphs()
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(px*c.s), float64(py*c.s))
	op.GeoM.Translate(float64(x*c.s), float64(y*c.s))
	op.ColorScale.ScaleWithColor(col)
	c.dst.DrawImage(glyphImages[code], op)
}

func rgb(hex uint32) color.RGBA {
	return color.RGBA{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 0xFF}
}

func rgba(hex uint32, a uint8) color.RGBA {
	c := rgb(hex)
	f := float32(a) / 255
	return color.RGBA{uint8(float32(c.R) * f), uint8(float32(c.G) * f), uint8(float32(c.B) * f), a}
}
