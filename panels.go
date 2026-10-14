package main

// The help and programs panels drawn over the window.

import (
	"strings"

	"odyssey9/machine"
)

func (g *game) drawPanel(c canvas, title string, w, h float32) (float32, float32) {
	c.rect(0, 0, g.width, baseH, rgba(0x000000, 0xB0))
	x := (baseW - w) / 2
	y := (baseH - h) / 2
	c.rrect(x, y, w, h, 12, rgb(0xF4F1EA))
	c.text(title, fontBold, 18, x+24, y+30, rgb(0x1A1A1A), -1)
	c.text("Esc or click to close", fontRegular, 12, x+w-24, y+30, rgb(0x777066), 1)
	return x, y
}

// sampleKeys are the keys that choose the manual's programs, in order.
const sampleKeys = "123456789ABCDEF"

// Size of the programs panel and of each program's row in it.
const (
	progPanelW = 640
	progRowH   = 38
)

func progPanelH() float32 {
	return float32(70 + progRowH*len(machine.Samples))
}

func (g *game) programRows() (x, y, w float32) {
	return (baseW - progPanelW) / 2, (baseH-progPanelH())/2 + 56, progPanelW
}

func (g *game) sampleAt(mx, my float32) int {
	x, y, w := g.programRows()
	if mx < x || mx > x+w || my < y {
		return -1
	}
	i := int((my - y) / progRowH)
	if i < len(machine.Samples) {
		return i
	}
	return -1
}

func (g *game) drawPrograms(c canvas) {
	g.drawPanel(c, "Programs from the manual", progPanelW, progPanelH())
	x, y, w := g.programRows()
	mx, my := g.cursor()
	hover := g.sampleAt(mx, my)
	for i, s := range machine.Samples {
		ry := y + float32(i)*progRowH
		if i == hover {
			c.rrect(x+12, ry, w-24, progRowH-4, 6, rgb(0xE2DCCD))
		}
		key := string(sampleKeys[i])
		c.text(key, fontBold, 14, x+30, ry+progRowH/2-2, rgb(0xC0392B), 0)
		c.text(s.Name, fontBold, 14, x+50, ry+12, rgb(0x1A1A1A), -1)
		c.text(s.Usage, fontRegular, 12, x+50, ry+27, rgb(0x5A554C), -1)
	}
}

var helpText = []string{
	"OPERATING THE COMPUTER  (see the manual's back gatefold)",
	"Power on shows SELECT GAME: press 1-4 for the built-in games",
	"   1 Creepy Crawler   2 Flash Card   3 Computer Telepathy   4 Between the Sheets",
	"RESET (▲) → COMMAND.   E execute · C continue after HLT · D display · P program",
	"PROGRAM: A assembler or M hex (machine) → I input · R roll",
	"INPUT: type an instruction (6B or LDV.B.00) and press ENTER.  CLEAR leaves.",
	"ROLL: U / D step up / down, CLEAR returns to PROGRAM mode.",
	"DISPLAY: 0-F registers · P program counter · S subroutine · X accumulator",
	"",
	"PC KEYBOARD",
	"Letters, digits, . ? + - = type directly   * → ×   / → ÷   space → SPACE",
	"[ → YES   ] → NO   Enter → ENTER   Delete → CLEAR   Esc → RESET",
	"Backspace erases while typing an instruction, else it is CLEAR",
	"",
	"EMULATOR",
	"F1 help · F2 save · F3 load · F4 programs · F5 speed · F6 monitor · F7 colours · F8 power",
	"Save/Load use the file given by -file. Drop a listing on the window to load it.",
	"Powering off erases the program, just like the real console.",
}

func (g *game) drawHelp(c canvas) {
	h := float32(70 + 22*len(helpText))
	x, y := g.drawPanel(c, "Help", 700, h)
	for i, line := range helpText {
		kind := fontRegular
		col := rgb(0x2A2722)
		// A heading is in capitals, though a note after it may not be.
		heading, _, _ := strings.Cut(line, "  (")
		if heading == strings.ToUpper(heading) && heading != "" && !strings.HasPrefix(heading, " ") {
			kind, col = fontBold, rgb(0xC0392B)
		}
		c.text(line, kind, 13, x+24, y+64+float32(i)*22, col, -1)
	}
}
