package main

// The monitor panel: registers and a disassembly of program memory.

import (
	"fmt"
	"slices"
	"unicode"

	"odyssey9/machine"
)

// listLine is one line of the monitor's listing: an instruction, or one
// step of data.
type listLine struct {
	step, n int    // the first step and how many steps
	text    string // the instruction, or the letter the data shows
}

// listing returns the lines of program memory: the instructions the
// program can reach (see machine.CodeSteps), and a line for each other
// step, which holds data, with its letter if it is one's character code.
func listing(mem *[machine.Steps]byte) []listLine {
	var lines []listLine
	code := machine.CodeSteps(mem)
	for s := 0; s < machine.Steps; {
		l := listLine{step: s, n: 1}
		if code[s] {
			l.text, l.n = machine.Disassemble(mem, s)
		} else if r, ok := machine.CodeRune(mem[s]); ok && unicode.IsLetter(r) {
			l.text = fmt.Sprintf("'%c'", r)
		}
		lines = append(lines, l)
		s += l.n
	}
	return lines
}

func (g *game) drawMonitor(c canvas) {
	x0 := float32(baseW)
	c.rect(x0, 0, monitorW, baseH, rgb(0x15171C))
	c.rect(x0, 0, 2, baseH, rgb(0x2E323B))
	m := g.m
	dim, ink, hi := rgb(0x7D8596), rgb(0xD6DBE6), rgb(0xFFD25A)
	y := float32(24)
	c.text("MONITOR", fontBold, 14, x0+20, y, ink, -1)
	if !g.power {
		c.text("OFF", fontMono, 12, x0+monitorW-20, y, dim, 1)
		return
	}
	c.text(m.Mode().String(), fontMono, 12, x0+monitorW-20, y, hi, 1)
	y += 30
	c.text(fmt.Sprintf("ACCUM %02X   PC %02d   SUBR %02d", m.A, m.PC, m.S), fontMono, 13, x0+20, y, ink, -1)
	y += 26
	for r := range 16 {
		cx := x0 + 20 + float32(r%4)*68
		cy := y + float32(r/4)*20
		col := ink
		if r == machine.RegB {
			col = rgb(0x8FD3FF)
		}
		c.text(fmt.Sprintf("R%X %02X", r, m.R[r]), fontMono, 13, cx, cy, col, -1)
	}
	y += 4*20 + 14
	c.rect(x0+16, y, monitorW-32, 1, rgb(0x2E323B))
	y += 16

	focus := m.PC
	label := "PC"
	if m.WaitingForKey() {
		focus = (m.PC + machine.Steps - 1) % machine.Steps // the INP/INA being executed
	}
	switch m.Mode() {
	case machine.ModeInput, machine.ModeRoll, machine.ModeProgram, machine.ModeAssembler, machine.ModeHex:
		focus, label = m.EditStep(), "STEP"
	}
	c.text("PROGRAM  (> "+label+")", fontBold, 12, x0+20, y, dim, -1)
	y += 20
	list := listing(&m.Mem)
	idx := 0
	for i, l := range list {
		if l.step <= focus {
			idx = i
		}
	}
	const lines = 30
	first := max(min(idx-lines/2, len(list)-lines), 0)
	for i := first; i < first+lines && i < len(list); i++ {
		l := list[i]
		code := m.Mem[l.step : l.step+l.n]
		col := dim
		if slices.ContainsFunc(code, func(b byte) bool { return b != 0 }) {
			col = ink
		}
		mark := " "
		if i == idx {
			mark = ">"
			col = hi
			c.rect(x0+12, y-9, monitorW-24, 18, rgb(0x2A2E38))
		}
		c.text(fmt.Sprintf("%s %02d  %-5s  %s", mark, l.step, machine.HexBytes(code), l.text), fontMono, 13, x0+18, y, col, -1)
		y += 19
	}
}
