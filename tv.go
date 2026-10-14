package main

// Drawing the TV, its screen and the buttons under it.

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"odyssey9/machine"
)

// charColor follows the colours of the "Computer Symbols" chart in the
// manual, which shade from purple through green and yellow to red.
func charColor(code byte) color.RGBA {
	palette := []uint32{0xC266DA, 0x9DB894, 0x52D163, 0xB2E23F, 0xF5C836, 0xF7613A, 0xEE3E45, 0xEE4580, 0xEE58B4}
	i := int(code) / 7
	if i >= len(palette) {
		i = len(palette) - 1
	}
	return rgb(palette[i])
}

const (
	tvX, tvY, tvW, tvH = 20, 16, 780, 388
	scrX, scrY         = 58, 40
	scrW, scrH         = 704, 300
)

func (g *game) Draw(screen *ebiten.Image) {
	c := canvas{screen, g.scale}
	screen.Fill(rgb(0x2A2622))
	g.drawTV(c)
	g.drawButtons(c)
	g.drawKeyboard(c)
	if g.monitor {
		g.drawMonitor(c)
	}
	switch g.overlay {
	case helpPanel:
		g.drawHelp(c)
	case programsPanel:
		g.drawPrograms(c)
	}
	g.drawToast(c)
	if g.shotNow {
		g.shotNow = false
		g.takeScreenshot(screen)
	}
}

func (g *game) drawTV(c canvas) {
	// cabinet and bezel
	c.rrect(tvX, tvY, tvW, tvH, 18, rgb(0x161412))
	c.rrect(tvX+8, tvY+8, tvW-16, tvH-16, 14, rgb(0x3B3631))
	c.rrect(scrX-10, scrY-10, scrW+20, scrH+20, 26, rgb(0x0C0B0A))
	bg := rgb(0x0A0F24)
	if !g.power {
		bg = rgb(0x060606)
	}
	c.rrect(scrX, scrY, scrW, scrH, 22, bg)

	if g.power {
		// The 11 output positions.
		const px, py = 7, 9
		const pitch = 62
		x0 := float32(scrX) + (scrW-(machine.Positions*pitch-6))/2
		y0 := float32(scrY + 70)
		for i, code := range g.m.Screen {
			x := x0 + float32(i)*pitch
			col := rgb(0xE6ECFF)
			if g.colors {
				col = charColor(code)
			}
			glow := color.RGBA{col.R / 5, col.G / 5, col.B / 5, 0x33}
			c.glyph(code, x-1.5, y0-1.5, px+3.0/8, py+3.0/7, glow)
			c.glyph(code, x, y0, px, py, col)
			c.text(fmt.Sprintf("%02X", i), fontMono, 10, x+28, y0+82, rgba(0x6C7AB0, 0x90), 0)
		}

		// The status line in the same character set.
		status := g.m.Status()
		const spx, spy, spitch = 3, 4, 28
		sx := float32(scrX) + (scrW-machine.StatusWidth*spitch)/2
		sy := float32(scrY + 205)
		codes := machine.TextCodes(status)
		for i, code := range codes {
			if i >= machine.StatusWidth {
				break
			}
			c.glyph(code, sx+float32(i)*spitch, sy, spx, spy, rgb(0xA9B8E8))
		}
		// blinking cursor at the end of the line being typed in Input mode
		// (not after a note such as ERROR)
		typing := g.m.Mode() == machine.ModeInput && strings.HasPrefix(status, "STEP ")
		if typing && g.frame/20%2 == 0 && len(codes) < machine.StatusWidth {
			c.glyph(0x2F, sx+float32(len(codes))*spitch, sy, spx, spy, rgba(0xE8ECFF, 0xA0))
		}
	}

	// scan lines
	for y := float32(scrY + 2); y < scrY+scrH-2; y += 3 {
		c.rect(scrX+4, y, scrW-8, 1, color.RGBA{0, 0, 0, 0x30})
	}

	// nameplate and power light
	c.text("ODYSSEY²", fontBold, 15, tvX+40, tvY+tvH-22, rgb(0xD8D2C4), -1)
	c.text("COMPUTER INTRO!  ·  VIDEOPAC 9", fontRegular, 12, tvX+140, tvY+tvH-22, rgb(0x9C958A), -1)
	led := rgb(0x401010)
	if g.power {
		led = rgb(0xFF3030)
	}
	c.circle(tvX+tvW-44, tvY+tvH-22, 6, led)
	c.text(machine.Speeds[g.m.Speed()].Name, fontMono, 11, tvX+tvW-60, tvY+tvH-22, rgb(0x9C958A), 1)
}

// drawToast shows the current message at the bottom of the TV screen, or
// under an open panel. A message too long to fit loses its middle, so the
// end of a file name stays readable.
func (g *game) drawToast(c canvas) {
	if g.toastFor <= 0 || g.toast == "" {
		return
	}
	const maxW = scrW - 20
	text := g.toast
	for r := []rune(text); c.textWidth(text, fontRegular, 13)+24 > maxW && len(r) > 1; {
		mid := len(r) / 2
		r = append(r[:mid], r[mid+1:]...)
		text = string(r[:mid]) + "…" + string(r[mid:])
	}
	w := c.textWidth(text, fontRegular, 13) + 24
	x, y := float32(scrX)+(scrW-w)/2, float32(scrY+scrH-38)
	if g.overlay != noPanel {
		x, y = (baseW-w)/2, baseH-46
	}
	c.rrect(x, y, w, 26, 8, rgba(0x000000, 0xD0))
	c.text(text, fontRegular, 13, x+w/2, y+13, rgb(0xFFFFFF), 0)
}

func (g *game) drawButtons(c canvas) {
	for _, b := range g.buttons {
		label := string(b.id)
		on := false
		switch b.id {
		case powerButton:
			on = g.power
		case speedButton:
			label = "SPEED: " + machine.Speeds[g.m.Speed()].Name
		case monitorButton:
			on = g.monitor
		case programsButton:
			on = g.overlay == programsPanel
		case helpButton:
			on = g.overlay == helpPanel
		}
		face := rgb(0x4A443D)
		if on {
			face = rgb(0x6E655A)
		}
		c.rrect(b.x, b.y, b.w, b.h, 6, rgb(0x14120F))
		c.rrect(b.x+1, b.y+1, b.w-2, b.h-3, 5, face)
		ink := rgb(0xEDE6D8)
		if b.id == powerButton && g.power {
			ink = rgb(0xFF7060)
		}
		c.text(label, fontBold, 12, b.x+b.w/2, b.y+b.h/2, ink, 0)
	}
}
