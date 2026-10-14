// Package machine emulates the virtual computer that the Odyssey2
// "Computer Intro!" cartridge (Videopac 9) provides: sixteen registers, an
// accumulator, 100 program steps, an 11-position screen, the operating
// modes described in the manual and the four built-in games.
package machine

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	// Steps is the number of program steps (00-99).
	Steps = 100
	// Positions is the number of character positions on the screen.
	Positions = 11
	// StatusWidth is the number of characters on the status line.
	StatusWidth = 18
	// RegB is the positioning register.
	RegB = 0x0B
	// RegC is the register used by the MOV instruction.
	RegC = 0x0C

	blank = 0x0C
)

// Mode is one of the operating modes from the manual's flow chart.
type Mode int

const (
	ModeSelect Mode = iota // power-on "SELECT GAME" screen
	ModeCommand
	ModeExecute
	ModeDisplay
	ModeProgram
	ModeAssembler
	ModeHex
	ModeInput
	ModeRoll
	ModeGame
)

var modeNames = map[Mode]string{
	ModeSelect: "SELECT", ModeCommand: "COMMAND", ModeExecute: "EXECUTION",
	ModeDisplay: "DISPLAY", ModeProgram: "PROGRAM", ModeAssembler: "ASSEMBLER",
	ModeHex: "HEX INPUT", ModeInput: "INPUT", ModeRoll: "ROLL", ModeGame: "GAME",
}

func (m Mode) String() string { return modeNames[m] }

// Sound is the tone currently being produced.
type Sound int

const (
	SoundNone  Sound = iota
	SoundBeep        // short key-confirmation beep
	SoundBuzz        // the one second SIG buzz
	SoundError       // low "bonk" for an invalid entry
	SoundChirp       // high blip used by the built-in games
	SoundBeeps       // a couple of quick beeps
)

// Speed presets in program steps executed per second.
var Speeds = []struct {
	Name string
	IPS  float64
}{
	{"SLOW", 4},
	{"NORMAL", 1000},
	{"FAST", 200000},
}

// CPU is the programmer-visible state of the virtual computer.
type CPU struct {
	Mem [Steps]byte // program steps 00-99 (RAM)
	R   [16]byte    // registers 0-F
	A   byte        // accumulator
	PC  int         // program counter, a step number 00-99
	S   int         // subroutine return register, a step number 00-99
}

// Machine is the complete emulated console with the cartridge inserted.
type Machine struct {
	CPU
	Screen [Positions]byte // character codes shown in the 11 positions

	mode  Mode
	speed int

	// program entry
	lang     Mode   // ModeAssembler or ModeHex: language for Input/Roll
	editStep int    // step shown in Input and Roll modes
	inBuf    []rune // characters typed in Input mode
	note     string // transient status message (e.g. "ERROR")
	noteLeft time.Duration

	// display mode
	display string

	// execution
	waitReg   int  // register awaiting a key (-1 = accumulator), or -2
	halted    bool // stopped by HLT
	budget    float64
	blockLeft time.Duration // remaining time the program is paused for SIG

	// sound
	sound     Sound
	soundLeft time.Duration

	game game
	rng  *rand.Rand
}

// New returns a machine that has just been switched on.
func New() *Machine {
	m := &Machine{speed: 1, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	m.PowerOn()
	return m
}

// PowerOn simulates switching the console off and on: the program and
// registers are erased and the title screen is shown.
func (m *Machine) PowerOn() {
	m.CPU = CPU{}
	m.clearScreen()
	m.mode = ModeSelect
	m.lang = ModeHex
	m.editStep = 0
	m.inBuf = nil
	m.display = ""
	m.waitReg = -2
	m.halted = false
	m.blockLeft = 0
	m.game = nil
	m.note = ""
	m.sound, m.soundLeft = SoundNone, 0
}

// Mode returns the current operating mode.
func (m *Machine) Mode() Mode { return m.mode }

// EditStep returns the step shown in the Input and Roll modes.
func (m *Machine) EditStep() int { return m.editStep }

// WaitingForKey reports whether a running program is waiting for input.
func (m *Machine) WaitingForKey() bool { return m.mode == ModeExecute && m.waitReg != -2 }

// Speed returns the index into Speeds of the current execution speed.
func (m *Machine) Speed() int { return m.speed }

// SetSpeed selects an entry from Speeds.
func (m *Machine) SetSpeed(i int) {
	if i >= 0 && i < len(Speeds) {
		m.speed = i
	}
}

// CurrentSound returns the tone that should be playing right now.
func (m *Machine) CurrentSound() Sound {
	if m.soundLeft > 0 {
		return m.sound
	}
	return SoundNone
}

func (m *Machine) play(s Sound, d time.Duration) {
	m.sound, m.soundLeft = s, d
}

func (m *Machine) clearScreen() {
	for i := range m.Screen {
		m.Screen[i] = blank
	}
}

func (m *Machine) setNote(s string) {
	m.note = s
	m.noteLeft = 1500 * time.Millisecond
}

func (m *Machine) errorBeep() {
	m.play(SoundError, 150*time.Millisecond)
}

// LoadProgram replaces program memory, clears the registers and returns to
// the Command mode.
func (m *Machine) LoadProgram(p *[Steps]byte) {
	m.PowerOn()
	m.Mem = *p
	m.mode = ModeCommand
}

// Status returns the text for the status line under the screen.
func (m *Machine) Status() string {
	if m.note != "" {
		return m.note
	}
	switch m.mode {
	case ModeSelect:
		return "SELECT GAME"
	case ModeCommand:
		if m.halted {
			return "HALT"
		}
		return "COMMAND"
	case ModeExecute:
		if m.waitReg != -2 {
			return "?"
		}
		return ""
	case ModeDisplay:
		if m.display == "" {
			return "DISPLAY"
		}
		return m.display
	case ModeProgram:
		return "PROGRAM"
	case ModeAssembler:
		return "ASSEMBLER"
	case ModeHex:
		return "HEX INPUT"
	case ModeInput:
		return fmt.Sprintf("STEP %02d %s", m.editStep, string(m.inBuf))
	case ModeRoll:
		return fmt.Sprintf("ROLL %02d  %02X", m.editStep, m.Mem[m.editStep])
	}
	return ""
}

// Press handles one key press.
func (m *Machine) Press(k Key) {
	if k == KeyReset {
		m.reset()
		return
	}
	m.note = ""
	switch m.mode {
	case ModeSelect:
		if d := k.Digit(); d >= 1 && d <= 4 {
			m.startGame(d)
		}
	case ModeCommand:
		m.pressCommand(k)
	case ModeExecute:
		m.pressExecute(k)
	case ModeDisplay:
		m.pressDisplay(k)
	case ModeProgram:
		switch k {
		case KeyA:
			m.mode, m.lang = ModeAssembler, ModeAssembler
		case KeyM:
			m.mode, m.lang = ModeHex, ModeHex
		case KeyClear:
			m.mode = ModeCommand
		default:
			m.errorBeep()
		}
	case ModeAssembler, ModeHex:
		switch k {
		case KeyI:
			m.mode = ModeInput
			m.inBuf = nil
		case KeyR:
			m.mode = ModeRoll
		case KeyClear:
			m.mode = ModeCommand
		default:
			m.errorBeep()
		}
	case ModeInput:
		m.pressInput(k)
	case ModeRoll:
		switch k {
		case KeyU:
			m.editStep = (m.editStep + 1) % Steps
		case KeyD:
			m.editStep = (m.editStep + Steps - 1) % Steps
		case KeyClear:
			m.mode = ModeProgram
		default:
			m.errorBeep()
		}
	case ModeGame:
		if m.game != nil {
			m.game.key(m, k)
		}
	}
}

// Backspace removes the last character typed in Input mode. The real
// keyboard has no such key; it is an emulator convenience.
func (m *Machine) Backspace() {
	if m.mode == ModeInput && len(m.inBuf) > 0 {
		m.inBuf = m.inBuf[:len(m.inBuf)-1]
	}
}

func (m *Machine) reset() {
	m.note = ""
	m.game = nil
	m.waitReg = -2
	m.blockLeft = 0
	m.inBuf = nil
	if m.mode == ModeGame || m.mode == ModeSelect {
		m.clearScreen()
	}
	m.mode = ModeCommand
}

func (m *Machine) pressCommand(k Key) {
	switch k {
	case KeyE:
		m.clearScreen()
		m.PC = 0
		m.halted = false
		m.startRunning()
	case KeyC:
		if m.halted && m.Mem[m.PC] == 0xFF {
			m.PC = (m.PC + 1) % Steps
		}
		m.halted = false
		m.startRunning()
	case KeyD:
		m.mode = ModeDisplay
		m.display = ""
	case KeyP:
		m.mode = ModeProgram
		m.editStep = 0
		m.halted = false
	case KeyClear:
	default:
		if d := k.Digit(); d >= 1 && d <= 4 {
			m.startGame(d)
			return
		}
		m.errorBeep()
	}
}

func (m *Machine) startRunning() {
	m.mode = ModeExecute
	m.waitReg = -2
	m.blockLeft = 0
	m.budget = 0
}

func (m *Machine) pressDisplay(k Key) {
	if k == KeyClear {
		m.mode = ModeCommand
		return
	}
	if r := k.HexDigit(); r >= 0 {
		m.display = fmt.Sprintf("REG %X  %02X", r, m.R[r])
		return
	}
	switch k {
	case KeyP:
		m.display = fmt.Sprintf("PROG CNTR  %02d", m.PC)
	case KeyS:
		m.display = fmt.Sprintf("SUBR CNTR  %02d", m.S)
	case KeyX:
		m.display = fmt.Sprintf("ACCUM  %02X", m.A)
	default:
		m.errorBeep()
	}
}

func (m *Machine) pressInput(k Key) {
	switch k {
	case KeyClear:
		m.mode = m.lang
		m.inBuf = nil
		return
	case KeyEnter:
		m.enterStatement()
		return
	}
	if m.lang == ModeHex {
		if k.HexDigit() < 0 {
			m.errorBeep()
			return
		}
		m.inBuf = append(m.inBuf, k.Rune())
		if len(m.inBuf) > 2 {
			m.inBuf = m.inBuf[len(m.inBuf)-2:]
		}
		return
	}
	r := k.Rune()
	if r == 0 || len(m.inBuf) >= StatusWidth-8 {
		m.errorBeep()
		return
	}
	m.inBuf = append(m.inBuf, r)
}

func (m *Machine) enterStatement() {
	var code []byte
	if m.lang == ModeHex {
		if len(m.inBuf) != 2 {
			m.errorBeep()
			return
		}
		var v byte
		fmt.Sscanf(string(m.inBuf), "%02X", &v)
		code = []byte{v}
	} else {
		var err error
		code, err = Assemble(string(m.inBuf))
		if err != nil {
			m.errorBeep()
			m.setNote("ERROR")
			m.inBuf = nil
			return
		}
	}
	for _, b := range code {
		m.Mem[m.editStep] = b
		m.editStep = (m.editStep + 1) % Steps
	}
	m.inBuf = nil
}

func (m *Machine) pressExecute(k Key) {
	if m.waitReg == -2 {
		return // the program only reads the keyboard at INP/INA
	}
	if m.waitReg == -1 {
		m.A = k.Code()
	} else {
		m.R[m.waitReg] = k.Code()
	}
	m.waitReg = -2
	m.play(SoundBeep, 60*time.Millisecond)
}

// Update advances the machine by dt of real time.
func (m *Machine) Update(dt time.Duration) {
	if m.soundLeft > 0 {
		m.soundLeft -= dt
	}
	if m.noteLeft > 0 {
		m.noteLeft -= dt
		if m.noteLeft <= 0 {
			m.note = ""
		}
	}
	switch m.mode {
	case ModeExecute:
		m.run(dt)
	case ModeGame:
		if m.game != nil {
			m.game.tick(m, dt)
		}
	}
}

func (m *Machine) run(dt time.Duration) {
	if m.blockLeft > 0 {
		m.blockLeft -= dt
		if m.blockLeft > 0 {
			return
		}
		dt = -m.blockLeft
		m.blockLeft = 0
	}
	ips := Speeds[m.speed].IPS
	m.budget += dt.Seconds() * ips
	if limit := ips / 10; m.budget > limit && limit >= 1 {
		m.budget = limit // don't try to catch up after a stall
	}
	for m.budget >= 1 && m.mode == ModeExecute && m.waitReg == -2 && m.blockLeft <= 0 {
		m.budget--
		m.Step()
	}
	if m.waitReg != -2 {
		m.budget = 0
	}
}

func (m *Machine) fetch() byte {
	b := m.Mem[m.PC]
	m.PC = (m.PC + 1) % Steps
	return b
}

func (m *Machine) output(code byte) {
	pos := int(m.R[RegB])
	if pos >= Positions {
		pos = 0
	}
	m.Screen[pos] = code
	pos++
	if pos >= Positions {
		pos = 0
	}
	m.R[RegB] = byte(pos)
}

// Step executes a single instruction.
func (m *Machine) Step() {
	start := m.PC
	op := m.fetch()
	r := int(op & 0x0F)
	switch {
	case op == 0x00: // NOP
	case op == 0x01: // CLR
		m.A = 0
	case op == 0x02: // DEC
		m.A = bcdSub(m.A, 1)
	case op == 0x03: // INC
		m.A = bcdAdd(m.A, 1)
	case op == 0x04: // INA
		m.waitReg = -1
	case op == 0x05: // SIG
		m.play(SoundBuzz, time.Second)
		m.blockLeft = time.Second
	case op == 0x07: // RET
		m.PC = m.S
	case op == 0x08: // RND
		m.A = stepToBCD(m.rng.Intn(100))
	case op == 0x09: // MOV: load A from the step addressed by register C
		m.A = m.Mem[bcdToStep(m.R[RegC])]
		m.R[RegC] = bcdAdd(m.R[RegC], 1)
	case op == 0x0B: // OTA
		m.output(m.A)
	case op >= 0x10 && op <= 0x14:
		target := bcdToStep(m.fetch())
		switch op {
		case 0x10: // BDB: branch if the high digit of A is 9
			if m.A>>4 == 9 {
				m.PC = target
			}
		case 0x11: // BDC: branch if the high digit of A is not 0
			if m.A>>4 != 0 {
				m.PC = target
			}
		case 0x12: // GTO
			m.PC = target
		case 0x13: // BRZ
			if m.A == 0 {
				m.PC = target
			}
		case 0x14: // GTS
			m.S = m.PC
			m.PC = target
		}
	case op >= 0x20 && op <= 0x6F:
		nn := m.fetch()
		target := bcdToStep(nn)
		reg := m.R[r]
		switch op >> 4 {
		case 2: // BNE
			if reg != m.A {
				m.PC = target
			}
		case 3: // BEQ
			if reg == m.A {
				m.PC = target
			}
		case 4: // BGT
			if reg > m.A {
				m.PC = target
			}
		case 5: // BLS
			if reg < m.A {
				m.PC = target
			}
		case 6: // LDV
			m.R[r] = nn
		}
	case op>>4 == 0x7: // INP
		m.waitReg = r
	case op>>4 == 0x8: // PAK
		m.A = m.R[r]&0x0F<<4 | m.R[(r+1)&0x0F]&0x0F
	case op>>4 == 0x9: // LDA
		m.A = m.R[r]
	case op>>4 == 0xA: // STO
		m.R[r] = m.A
	case op>>4 == 0xB: // UNP
		m.R[r] = m.A >> 4
		m.R[(r+1)&0x0F] = m.A & 0x0F
	case op>>4 == 0xC: // OUT
		m.output(m.R[r])
	case op>>4 == 0xD: // SUB: A = register - A
		m.A = bcdSub(m.R[r], m.A)
	case op>>4 == 0xE: // ADD
		m.A = bcdAdd(m.A, m.R[r])
	case op == 0xFF: // HLT
		m.PC = start
		m.halted = true
		m.mode = ModeCommand
	default:
		// Undefined opcodes behave as NOP.
	}
}
