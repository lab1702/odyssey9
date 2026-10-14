package main

// The monitor panel: registers and a disassembly of program memory.

import (
	"fmt"

	"odyssey9/machine"
)

// listing returns instruction start steps, decoding from step 00.
func listing(mem *[machine.Steps]byte) []int {
	var starts []int
	for s := 0; s < machine.Steps; {
		starts = append(starts, s)
		_, n := machine.Disassemble(mem, s)
		s += n
	}
	return starts
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
	starts := listing(&m.Mem)
	idx := 0
	for i, s := range starts {
		if s <= focus {
			idx = i
		}
	}
	const lines = 30
	first := max(min(idx-lines/2, len(starts)-lines), 0)
	for i := first; i < first+lines && i < len(starts); i++ {
		s := starts[i]
		text, n := machine.Disassemble(&m.Mem, s)
		hex := fmt.Sprintf("%02X", m.Mem[s])
		if n == 2 {
			hex += fmt.Sprintf(" %02X", m.Mem[s+1])
		}
		col := dim
		if m.Mem[s] != 0 || (n == 2 && m.Mem[s+1] != 0) {
			col = ink
		}
		mark := " "
		if i == idx {
			mark = ">"
			col = hi
			c.rect(x0+12, y-9, monitorW-24, 18, rgb(0x2A2E38))
		}
		c.text(fmt.Sprintf("%s %02d  %-5s  %s", mark, s, hex, text), fontMono, 13, x0+18, y, col, -1)
		y += 19
	}
}
