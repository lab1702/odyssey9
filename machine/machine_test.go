package machine

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

// screenText renders the 11 screen positions as ASCII for assertions.
func screenText(m *Machine) string {
	var sb strings.Builder
	for _, c := range m.Screen {
		switch {
		case c <= 9:
			sb.WriteByte('0' + c)
		case c == blank:
			sb.WriteByte(' ')
		default:
			if r, ok := codeLetters[c]; ok {
				sb.WriteRune(r)
				continue
			}
			sym := map[byte]byte{0x10: '+', 0x29: '*', 0x2A: '/', 0x2B: '=', 0x27: '.', 0x0D: '?'}
			if s, ok := sym[c]; ok {
				sb.WriteByte(s)
			} else {
				sb.WriteByte('#')
			}
		}
	}
	return sb.String()
}

// runUntilInput executes until the program waits for a key.
func runUntilInput(t *testing.T, m *Machine) {
	t.Helper()
	for range 200000 {
		if m.WaitingForKey() || m.mode != ModeExecute {
			return
		}
		m.blockLeft = 0 // skip SIG delays
		m.Step()
	}
	t.Fatalf("program never asked for input (PC=%02d)", m.PC)
}

func runSample(t *testing.T, name string, keys ...Key) *Machine {
	t.Helper()
	m := New()
	for _, s := range Samples {
		if s.Name == name {
			m.LoadProgram(s.Program())
		}
	}
	m.Press(KeyE)
	runUntilInput(t, m)
	for _, k := range keys {
		m.Press(k)
		runUntilInput(t, m)
	}
	return m
}

func TestBCD(t *testing.T) {
	cases := []struct{ a, b, want byte }{
		{0x09, 0x09, 0x18}, {0x08, 0x08, 0x16}, {0x13, 0x09, 0x22},
		{0x90, 0x02, 0x92}, {0x99, 0x01, 0x00}, {0x16, 0x08, 0x24},
	}
	for _, c := range cases {
		if got := bcdAdd(c.a, c.b); got != c.want {
			t.Errorf("bcdAdd(%02X,%02X)=%02X want %02X", c.a, c.b, got, c.want)
		}
	}
	if got := bcdSub(0x12, 0x02); got != 0x10 {
		t.Errorf("12-02=%02X", got)
	}
	if got := bcdSub(0x00, 0x01); got != 0x99 {
		t.Errorf("00-01=%02X", got)
	}
	if got := bcdSub(0x0B, 0x01); got != 0x0A {
		t.Errorf("0B-01=%02X", got)
	}
}

func TestAssembleRoundTrip(t *testing.T) {
	for _, src := range []string{"LDV.B.00", "INP.0", "INA", "GTO.15", "BNE.4.24", "HLT", "UNP.F", "GTS.66"} {
		code, err := Assemble(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		var mem [Steps]byte
		copy(mem[:], code)
		text, n := Disassemble(&mem, 0)
		if text != src || n != len(code) {
			t.Errorf("%s -> % X -> %s (%d)", src, code, text, n)
		}
	}
	for _, bad := range []string{"LDV.B", "XYZ", "OUT.10", "GTO.123", "GTO.A5", "BNE.4.2F", "GTS.FF"} {
		if _, err := Assemble(bad); err == nil {
			t.Errorf("%s assembled without error", bad)
		}
	}
	if code, _ := Assemble("BVNE.4.24"); !bytes.Equal(code, []byte{0x24, 0x24}) {
		t.Errorf("BVNE alias: % X", code)
	}
	if code, err := Assemble("LDV.9.2A"); err != nil || !bytes.Equal(code, []byte{0x69, 0x2A}) {
		t.Errorf("LDV with a hex value: % X, %v", code, err)
	}
}

// A two-byte instruction at step 99 executes with its operand from step
// 00, but listings show that byte on step 00's own line.
func TestInstructionAtStep99(t *testing.T) {
	var mem [Steps]byte
	mem[0], mem[99] = 0x15, opGTO
	if text, n := Disassemble(&mem, 99); text != "GTO.15" || n != 1 {
		t.Errorf("Disassemble at 99 = %q, %d", text, n)
	}
	var buf bytes.Buffer
	FormatProgram(&buf, "t", &mem)
	if !strings.Contains(buf.String(), "\n99    12     GTO.15\n") {
		t.Errorf("listing:\n%s", buf.String())
	}
	m := New()
	m.LoadProgram(&mem)
	m.PC = 99
	m.Step()
	if m.PC != 15 {
		t.Errorf("GTO at step 99 went to %02d", m.PC)
	}
}

// Disassemble takes any step number, counting round from 00 to 99.
func TestDisassembleWrapsStep(t *testing.T) {
	var mem [Steps]byte
	mem[99] = opCLR
	for _, step := range []int{99, -1, 199} {
		if s, n := Disassemble(&mem, step); s != "CLR" || n != 1 {
			t.Errorf("Disassemble at step %d: %q, %d steps", step, s, n)
		}
	}
}

// A branch target byte that isn't a step number is shown with the step it
// really goes to, and the listing still loads back byte for byte.
func TestDisassembleBadTarget(t *testing.T) {
	var mem [Steps]byte
	copy(mem[:], []byte{opGTO, 0xA5, opBEQ | 2, 0x1A, opLDV | 9, 0x2A})
	for _, c := range []struct {
		step int
		want string
	}{{0, "GTO.A5 (=05)"}, {2, "BEQ.2.1A (=20)"}, {4, "LDV.9.2A"}} {
		if text, _ := Disassemble(&mem, c.step); text != c.want {
			t.Errorf("step %02d: %q, want %q", c.step, text, c.want)
		}
	}
	var buf bytes.Buffer
	FormatProgram(&buf, "t", &mem)
	q, err := ParseProgram(&buf)
	if err != nil || *q != mem {
		t.Errorf("round trip: %v\n%s", err, buf.String())
	}
	m := New()
	m.LoadProgram(&mem)
	m.Step()
	if m.PC != 5 {
		t.Errorf("GTO.A5 went to %02d", m.PC)
	}
}

// Every assembler statement printed in the manual's listings must assemble
// to the hex code printed beside it.
func TestSampleMnemonics(t *testing.T) {
	n := 0
	for _, s := range Samples {
		for line := range strings.SplitSeq(s.Listing, "\n") {
			fields := strings.Fields(line)
			var code []byte
			i := 1
			for ; i < len(fields) && len(fields[i]) == 2; i++ {
				var b byte
				if _, err := fmt.Sscanf(fields[i], "%02X", &b); err != nil {
					break
				}
				code = append(code, b)
			}
			if i >= len(fields) {
				continue
			}
			if _, ok := opByMnemonic[strings.Split(fields[i], ".")[0]]; !ok {
				continue // data and remarks
			}
			got, err := Assemble(fields[i])
			if err != nil || !bytes.Equal(got, code) {
				t.Errorf("%s: %q assembles to % X, %v; listing has % X", s.Name, fields[i], got, err, code)
			}
			n++
		}
	}
	if n < 300 {
		t.Errorf("only %d statements checked", n)
	}
}

func TestAdditionA(t *testing.T) {
	m := runSample(t, "Addition A", Key9, Key9)
	if got := screenText(m); got != "18         " {
		t.Errorf("9+9 shows %q", got)
	}
}

func TestAdditionB(t *testing.T) {
	m := runSample(t, "Addition B", Key2, Key4)
	if got := screenText(m); got != "2+4=06     " {
		t.Errorf("got %q", got)
	}
}

func TestAdditionC(t *testing.T) {
	m := runSample(t, "Addition C", Key7, Key8)
	if got := screenText(m); got != "7+8=15     " {
		t.Errorf("got %q", got)
	}
	m.Press(Key3) // next problem: old one is erased
	runUntilInput(t, m)
	if got := screenText(m); got != "3+         " {
		t.Errorf("after erase got %q", got)
	}
}

// The manual's three ways to enter and output a letter.
func TestEnterALetter(t *testing.T) {
	for name, keys := range map[string][]Key{
		"Enter a Letter A": nil,
		"Enter a Letter B": {KeyH},
		"Enter a Letter C": {KeyH},
	} {
		m := runSample(t, name, keys...)
		if got := screenText(m); got != "H          " {
			t.Errorf("%s shows %q", name, got)
		}
	}
}

func TestMultiplication(t *testing.T) {
	m := runSample(t, "Multiplication", Key3, Key7)
	if got := screenText(m); got != "3*7=21     " {
		t.Errorf("got %q", got)
	}
}

func TestDivision(t *testing.T) {
	m := runSample(t, "Division", Key6, Key2)
	if got := screenText(m); got != "6/2=03     " {
		t.Errorf("6/2 got %q", got)
	}
	m = runSample(t, "Division", Key9, Key2)
	if got := screenText(m); got != "9/2=04+R   " {
		t.Errorf("9/2 got %q", got)
	}
}

func TestArea(t *testing.T) {
	m := runSample(t, "Area (Subroutine)", Key8, Key3, Key1)
	if got := screenText(m); got != "24         " {
		t.Errorf("rectangle 8x3 got %q", got)
	}
	m = runSample(t, "Area (Subroutine)", Key6, Key2, Key2)
	if got := screenText(m); got != "06         " {
		t.Errorf("triangle 6x2/2 got %q", got)
	}
	m = runSample(t, "Area (Subroutine)", Key5, Key3, Key2)
	if got := screenText(m); got != "07+R       " {
		t.Errorf("triangle 5x3/2 got %q", got)
	}
}

func TestMessage(t *testing.T) {
	m := runSample(t, "Message", Key2)
	if got := screenText(m); got != "TAKE A LOOK" {
		t.Errorf("message 2 got %q", got)
	}
	m.Press(Key1)
	runUntilInput(t, m)
	if got := screenText(m); got != "HELLO      " {
		t.Errorf("message 1 got %q", got)
	}
}

func TestSixLetterGuess(t *testing.T) {
	word := []Key{KeyP, KeyU, KeyZ, KeyZ, KeyL, KeyE}
	m := runSample(t, "Six Letter Guess", word...)
	if got := screenText(m); got != "......     " {
		t.Errorf("after word got %q", got)
	}
	m.Press(KeyZ)
	runUntilInput(t, m)
	if got := screenText(m); got != "..ZZ..     " {
		t.Errorf("after Z got %q", got)
	}
	for _, k := range []Key{KeyP, KeyU, KeyL, KeyE} {
		m.Press(k)
		runUntilInput(t, m)
	}
	if got := screenText(m); got != "PUZZLE     " {
		t.Errorf("solved got %q", got)
	}
}

func TestFlashCardProgram(t *testing.T) {
	m := runSample(t, "Flash Card")
	sum := m.R[0] + m.R[1]
	m.Press(Key(int(Key0) + int(sum/10)))
	runUntilInput(t, m)
	m.Press(Key(int(Key0) + int(sum%10)))
	runUntilInput(t, m)
	// A correct answer plays the tune, erases the screen and asks again.
	if strings.Contains(screenText(m), "NO") {
		t.Errorf("correct answer rejected: %q", screenText(m))
	}
	want := fmt.Sprintf("%d+%d=       ", m.R[0], m.R[1])
	if got := screenText(m); got != want || m.Mode() != ModeExecute || !m.WaitingForKey() {
		t.Errorf("after a right answer: screen %q (want %q), mode %v", got, want, m.Mode())
	}
	// A wrong answer gets a NO.
	sum = m.R[0] + m.R[1]
	m.Press(Key(int(Key0) + int(sum/10)))
	runUntilInput(t, m)
	m.Press(Key(int(Key0) + int(sum%10+1)%10))
	runUntilInput(t, m)
	if !strings.Contains(screenText(m), "NO") {
		t.Errorf("wrong answer accepted: %q", screenText(m))
	}
}

// The original Creepy Crawler piles little men up around the screen.
func TestCreepyCrawler(t *testing.T) {
	m := New()
	m.rng = rand.New(rand.NewSource(1)) // some leave only two men after 500 steps
	m.LoadProgram(Samples[0].Program())
	m.Press(KeyE)
	buzzed := false
	for range 500 {
		m.blockLeft = 0
		m.Step()
		buzzed = buzzed || m.CurrentSound() == SoundBuzz
	}
	men := 0
	for _, c := range m.Screen {
		switch c {
		case 0x3A:
			men++
		case blank:
		default:
			t.Fatalf("unexpected symbol %02X on screen", c)
		}
	}
	if men < 3 || !buzzed {
		t.Errorf("%d little men on screen, buzzed %v; want them piled up with a buzz", men, buzzed)
	}
}

func TestCreepyCrawlerII(t *testing.T) {
	m := New()
	m.LoadProgram(Samples[1].Program())
	m.Press(KeyE)
	for range 500 {
		m.blockLeft = 0
		m.Step()
	}
	symbols := map[byte]bool{0x32: true, 0x33: true, 0x3A: true, 0x34: true, 0x35: true,
		0x37: true, 0x3D: true, 0x3E: true, 0x36: true, 0x3C: true, blank: true}
	shown := 0
	for _, c := range m.Screen {
		if !symbols[c] {
			t.Fatalf("unexpected symbol %02X on screen", c)
		}
		if c != blank {
			shown++
		}
	}
	if shown == 0 {
		t.Errorf("screen is blank")
	}
}

func TestHexEntryAndRoll(t *testing.T) {
	m := New()
	for _, k := range []Key{KeyReset, KeyP, KeyM, KeyI, Key6, Key0, KeyEnter, Key3, KeyA, KeyEnter} {
		m.Press(k)
	}
	if m.Mem[0] != 0x60 || m.Mem[1] != 0x3A || m.editStep != 2 {
		t.Fatalf("mem % X step %d", m.Mem[:3], m.editStep)
	}
	// Switch to assembler at step 02 via Clear, Roll, Clear, A, I.
	for _, k := range []Key{KeyClear, KeyR, KeyClear, KeyA, KeyI} {
		m.Press(k)
	}
	for _, r := range "LDV.B.00" {
		if r == '.' {
			m.Press(KeyPeriod)
		} else {
			m.Press(KeyForLetter(r))
		}
	}
	m.Press(KeyEnter)
	if m.Mem[2] != 0x6B || m.Mem[3] != 0x00 || m.editStep != 4 {
		t.Fatalf("assembler entry: % X step %d", m.Mem[:5], m.editStep)
	}
	m.Press(KeyClear)
	m.Press(KeyR)
	m.Press(KeyD)
	if got := m.Status(); got != "ROLL 03  00" {
		t.Errorf("roll status %q", got)
	}
}

// Entering step 99 moves on to step 00, and Roll wraps both ways.
func TestEditingWrapsAtStep99(t *testing.T) {
	m := New()
	press(m, KeyReset, KeyP, KeyM, KeyR, KeyD)
	if got := m.Status(); got != "ROLL 99  00" {
		t.Errorf("Roll down from step 00: %q", got)
	}
	press(m, KeyClear, KeyM, KeyI, KeyF, KeyF, KeyEnter)
	if m.Mem[99] != 0xFF || m.editStep != 0 {
		t.Errorf("hex entry at step 99: step 99 = %02X, next step %02d", m.Mem[99], m.editStep)
	}
	press(m, KeyClear, KeyR, KeyD, KeyU)
	if got := m.Status(); got != "ROLL 00  00" {
		t.Errorf("Roll up from step 99: %q", got)
	}
}

func TestHaltDisplayContinue(t *testing.T) {
	m := New()
	prog, _ := ParseProgram(strings.NewReader("00 60 05\n02 FF\n03 61 07\n05 FF"))
	m.LoadProgram(prog)
	m.Press(KeyE)
	m.Update(time.Second)
	if m.Mode() != ModeCommand || m.Status() != "HALT" {
		t.Fatalf("mode %v status %q", m.Mode(), m.Status())
	}
	m.Press(KeyD)
	m.Press(KeyP)
	if got := m.Status(); got != "PROG CNTR  02" {
		t.Errorf("PC display %q", got)
	}
	m.Press(Key0)
	if got := m.Status(); got != "REG 0  05" {
		t.Errorf("R0 display %q", got)
	}
	m.Press(KeyClear)
	m.Press(KeyC)
	m.Update(time.Second)
	if m.R[1] != 0x07 || m.PC != 5 {
		t.Errorf("continue: R1=%02X PC=%d", m.R[1], m.PC)
	}
}

// C continues after the HLT even when Program mode or a game was visited
// in between.
func TestHaltContinueAfterDetour(t *testing.T) {
	for name, detour := range map[string][]Key{
		"program": {KeyP, KeyM, KeyR, KeyClear, KeyClear},
		"game":    {Key1, KeyReset},
	} {
		m := New()
		m.LoadProgram(&[Steps]byte{0x03, 0xFF, 0x03, 0xFF}) // INC, HLT, INC, HLT
		m.Press(KeyE)
		m.Update(time.Second)
		if m.PC != 1 || m.Status() != "HALT" {
			t.Fatalf("%s: first halt PC=%02d status %q", name, m.PC, m.Status())
		}
		for _, k := range detour {
			m.Press(k)
		}
		if m.Mode() != ModeCommand || m.Status() != "COMMAND" {
			t.Errorf("%s: after detour mode %v status %q", name, m.Mode(), m.Status())
		}
		m.Press(KeyC)
		m.Update(time.Second)
		if m.PC != 3 || m.A != 0x02 || m.Status() != "HALT" {
			t.Errorf("%s: continue PC=%02d A=%02X status %q", name, m.PC, m.A, m.Status())
		}
	}
}

// RESET while the program waits at INP/INA, then C, waits for the key
// again instead of skipping the input.
func TestResetDuringInputContinue(t *testing.T) {
	for _, op := range []byte{opINP, opINA} {
		var prog [Steps]byte
		prog[0], prog[1], prog[99] = opGTO, 0x99, op // GTO.99; INP.0 or INA at step 99
		m := New()
		m.LoadProgram(&prog)
		m.Press(KeyE)
		m.Update(time.Second)
		if !m.WaitingForKey() {
			t.Fatalf("%02X: not waiting: PC=%02d mode %v", op, m.PC, m.Mode())
		}
		m.Press(KeyReset)
		m.Press(KeyD)
		m.Press(KeyP)
		if got := m.Status(); got != "PROG CNTR  99" {
			t.Errorf("%02X: PC display %q", op, got)
		}
		m.Press(KeyClear)
		m.Press(KeyC)
		m.Update(time.Second)
		if !m.WaitingForKey() {
			t.Fatalf("%02X: C skipped the input: PC=%02d mode %v", op, m.PC, m.Mode())
		}
		m.Press(Key5)
		if got := map[byte]byte{opINP: m.R[0], opINA: m.A}[op]; got != 5 {
			t.Errorf("%02X: read %02X after pressing 5", op, got)
		}
	}
}

// A two-byte statement entered at step 99 is refused rather than putting
// its operand in step 00; a one-byte statement still fits.
func TestAssemblerEntryAtStep99(t *testing.T) {
	m := New()
	for _, k := range []Key{KeyReset, KeyP, KeyA, KeyR, KeyD, KeyClear, KeyA, KeyI} {
		m.Press(k)
	}
	if m.Mode() != ModeInput || m.EditStep() != 99 {
		t.Fatalf("mode %v step %02d", m.Mode(), m.EditStep())
	}
	typeText := func(s string) {
		for _, r := range s {
			if r == '.' {
				m.Press(KeyPeriod)
			} else {
				m.Press(KeyForLetter(r))
			}
		}
		m.Press(KeyEnter)
	}
	typeText("GTO.15")
	if m.Status() != "ERROR" || m.Mem[99] != 0 || m.Mem[0] != 0 || m.EditStep() != 99 {
		t.Errorf("GTO at 99: status %q mem[99]=%02X mem[0]=%02X step %02d",
			m.Status(), m.Mem[99], m.Mem[0], m.EditStep())
	}
	typeText("INC")
	if m.Mem[99] != opINC || m.EditStep() != 0 {
		t.Errorf("INC at 99: mem[99]=%02X step %02d", m.Mem[99], m.EditStep())
	}
}

func TestRND(t *testing.T) {
	m := New()
	m.LoadProgram(&[Steps]byte{opRND})
	seen := map[byte]bool{}
	for range 1000 {
		m.PC = 0
		m.Step()
		if m.A>>4 > 9 || m.A&0x0F > 9 {
			t.Fatalf("RND gave %02X, not a decimal 00-99", m.A)
		}
		seen[m.A] = true
	}
	if len(seen) < 2 {
		t.Errorf("RND gave only %d distinct values in 1000 draws", len(seen))
	}
}

func TestProgramFileRoundTrip(t *testing.T) {
	for _, s := range Samples {
		p := s.Program()
		var buf bytes.Buffer
		if err := FormatProgram(&buf, s.Name, p); err != nil {
			t.Fatal(err)
		}
		q, err := ParseProgram(&buf)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		if *p != *q {
			t.Errorf("%s: round trip mismatch", s.Name)
		}
	}
}

func TestEmptyProgramRoundTrip(t *testing.T) {
	var empty [Steps]byte
	var buf bytes.Buffer
	if err := FormatProgram(&buf, "Empty", &empty); err != nil {
		t.Fatal(err)
	}
	q, err := ParseProgram(&buf)
	if err != nil {
		t.Fatalf("empty program does not load back: %v", err)
	}
	if *q != empty {
		t.Errorf("empty program round trip mismatch")
	}
}

func TestParseProgramBOM(t *testing.T) {
	p, err := ParseProgram(strings.NewReader("\xEF\xBB\xBF00 60 05\n02 FF\n"))
	if err != nil {
		t.Fatalf("listing with byte order mark: %v", err)
	}
	if p[0] != 0x60 || p[1] != 0x05 || p[2] != 0xFF {
		t.Errorf("got % X", p[:3])
	}
}

// A remark that starts with a hex-like word must not be read as code once
// it is set apart from the bytes by a wider gap.
func TestParseProgramRemark(t *testing.T) {
	p, err := ParseProgram(strings.NewReader("05 C0  BE QUIET\n10\t07 BE\tAD\n20 6B 00 LDV.B.00\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]byte{5: 0xC0, 6: 0, 10: 0x07, 11: 0xBE, 12: 0, 20: 0x6B, 21: 0x00}
	for step, b := range want {
		if p[step] != b {
			t.Errorf("step %02d = %02X, want %02X", step, p[step], b)
		}
	}
	if _, err := ParseProgram(strings.NewReader("05  QUIET\n")); err == nil {
		t.Errorf("a step with no hex code was accepted")
	}
}

// RESET must silence the SIG buzz along with stopping the program.
func TestResetStopsSound(t *testing.T) {
	var prog [Steps]byte
	prog[0] = 0x05 // SIG
	m := New()
	m.LoadProgram(&prog)
	m.Press(KeyE)
	m.Update(time.Second / 60)
	if m.CurrentSound() != SoundBuzz {
		t.Fatalf("SIG didn't buzz")
	}
	m.Press(KeyReset)
	if s := m.CurrentSound(); s != SoundNone {
		t.Errorf("sound %d still playing after RESET", s)
	}
}

// Switching to SLOW during a SIG buzz doesn't let the program run ahead
// once the buzz ends.
func TestSpeedChangeDuringSig(t *testing.T) {
	var prog [Steps]byte
	prog[0] = 0x05 // SIG
	for i := 1; i < Steps; i++ {
		prog[i] = 0x03 // INC
	}
	m := New()
	m.LoadProgram(&prog)
	m.SetSpeed(SpeedFast)
	m.Press(KeyE)
	frame := time.Second / 60
	m.Update(frame)
	m.SetSpeed(SpeedSlow)
	for range 66 {
		m.Update(frame)
	}
	if m.A > 0x01 {
		t.Errorf("A=%02X: program ran ahead after switching to SLOW", m.A)
	}
}

// Steps owed for the frame in which SIG ran must not run all at once when
// its second is up.
func TestNoBurstAfterSig(t *testing.T) {
	var prog [Steps]byte
	prog[0] = 0x05 // SIG
	for i := 1; i < Steps; i++ {
		prog[i] = 0x03 // INC
	}
	m := New()
	m.LoadProgram(&prog)
	m.SetSpeed(SpeedNormal)
	m.Press(KeyE)
	m.Update(time.Second / 60)
	m.Update(time.Second)
	if m.PC != 1 {
		t.Errorf("PC=%02d right after the SIG second; want 01", m.PC)
	}
}

// Time going backwards doesn't lengthen a sound or SIG pause.
func TestNegativeUpdate(t *testing.T) {
	var prog [Steps]byte
	prog[0] = 0x05 // SIG
	m := New()
	m.LoadProgram(&prog)
	m.Press(KeyE)
	m.Update(time.Second / 60) // runs SIG
	m.Update(-time.Hour)
	if left := m.SoundLeft(); left > time.Second {
		t.Errorf("SIG sound has %v left after a negative update", left)
	}
}

func TestBuiltInGames(t *testing.T) {
	m := New()
	m.Press(Key3) // Computer Telepathy from the title screen
	g := m.game.(*telepathy)
	g.secret = 42
	m.Press(Key5)
	m.Press(Key0)
	if got := screenText(m); got != "50 H       " {
		t.Errorf("telepathy high got %q", got)
	}
	m.Press(Key4)
	m.Press(Key2)
	if got := screenText(m); got != "42 X       " {
		t.Errorf("telepathy win got %q", got)
	}

	m.Press(KeyReset)
	m.Press(Key4)
	b := m.game.(*betweenSheets)
	b.lo, b.hi, b.secret = 3, 7, 5
	m.Press(KeyYes)
	if got := screenText(m); got != "03 05 07 01" {
		t.Errorf("between got %q", got)
	}
	// A right NO shows the secret in its place outside the sheets.
	b.lo, b.hi, b.secret, b.waiting = 3, 7, 50, true
	m.Press(KeyNo)
	if got := screenText(m); got != "03 07 50 02" {
		t.Errorf("between NO got %q", got)
	}
	// Y and N share YES and NO's codes, so they answer too; other keys
	// don't.
	b.lo, b.hi, b.secret, b.waiting = 3, 7, 5, true
	m.Press(KeyA)
	if !b.waiting || b.score != 2 {
		t.Errorf("A answered: waiting %v, score %d", b.waiting, b.score)
	}
	m.Press(KeyY)
	if got := screenText(m); got != "03 05 07 03" {
		t.Errorf("between Y got %q", got)
	}
	b.lo, b.hi, b.secret, b.waiting = 3, 7, 50, true
	m.Press(KeyN)
	if got := screenText(m); got != "03 07 50 04" {
		t.Errorf("between N got %q", got)
	}
}

// Between the Sheets: YES is right about half the time, and the secret is
// never one of the sheets.
func TestBetweenSheetsOdds(t *testing.T) {
	m := New()
	m.Press(Key4)
	g := m.game.(*betweenSheets)
	inside := 0
	for range 10000 {
		g.newRound(m)
		if g.secret == g.lo || g.secret == g.hi || g.secret < 0 || g.secret > 99 {
			t.Fatalf("sheets %02d %02d, secret %d", g.lo, g.hi, g.secret)
		}
		if g.secret > g.lo && g.secret < g.hi {
			inside++
		}
	}
	if inside < 4500 || inside > 5500 {
		t.Errorf("secret between the sheets %d times in 10000", inside)
	}
}

// Flash Card's problem is shown as soon as it is set, not after a blink.
func TestFlashCardShowsProblem(t *testing.T) {
	m := New()
	m.Press(Key2)
	m.Update(100 * time.Millisecond)
	if got := screenText(m); strings.TrimSpace(got) == "" {
		t.Errorf("new problem not shown: %q", got)
	}
}

// Flash Card: a wrong answer gets an angry NO, which the next digit
// clears; a right one stays on the screen, no longer blinking, until any
// key brings the next problem.
func TestFlashCardGame(t *testing.T) {
	m := New()
	m.Press(Key2)
	g := m.game.(*flashCard)
	g.a, g.b = 3, 4
	g.drawProblem(m, true)
	m.Press(Key0)
	m.Press(Key8)
	if got := screenText(m); got != "3+4=08 NO  " || m.CurrentSound() != SoundError {
		t.Errorf("wrong answer: screen %q, sound %v", got, m.CurrentSound())
	}
	m.Press(KeyA) // not a digit
	if got := screenText(m); got != "3+4=08 NO  " || len(g.answer) != 0 {
		t.Errorf("A taken as an answer: screen %q", got)
	}
	m.Press(Key0)
	if got := screenText(m); got != "3+4=0      " {
		t.Errorf("next digit didn't clear the NO: %q", got)
	}
	m.Press(Key7)
	if got := screenText(m); got != "3+4=07     " || !g.solved || m.CurrentSound() != SoundChirp {
		t.Errorf("right answer: screen %q, solved %v, sound %v", got, g.solved, m.CurrentSound())
	}
	for range 120 { // 2 s: the solved problem doesn't blink
		m.Update(time.Second / 60)
		if got := screenText(m); got != "3+4=07     " {
			t.Fatalf("solved problem changed to %q", got)
		}
	}
	m.Press(KeyA)
	if g.solved || len(g.answer) != 0 || strings.TrimSpace(screenText(m)[4:]) != "" {
		t.Errorf("any key didn't bring a new problem: %q", screenText(m))
	}
}

// The built-in Creepy Crawler flashes blocks on and off at random with a
// buzz, and ignores the keys.
func TestCreepyCrawlerGame(t *testing.T) {
	m := New()
	m.Press(Key1)
	buzzed, on, off := false, 0, 0
	for range 600 { // 10 s
		before := m.Screen
		m.Press(Key5)
		m.Update(time.Second / 60)
		buzzed = buzzed || m.CurrentSound() == SoundBuzz
		for i, c := range m.Screen {
			switch {
			case c != blank && c != 0x2F:
				t.Fatalf("unexpected symbol %02X on screen", c)
			case c == before[i]:
			case c == 0x2F:
				on++
			default:
				off++
			}
		}
	}
	if on == 0 || off == 0 || !buzzed || m.Mode() != ModeGame {
		t.Errorf("blocks on %d times and off %d, buzzed %v, mode %v", on, off, buzzed, m.Mode())
	}
}

// sampleFile returns the file name and text of sample i as a listing in
// examples/.
func sampleFile(i int, s Sample) (name, text string) {
	name = fmt.Sprintf("%02d-%s.txt", i+1, strings.Map(func(r rune) rune {
		if r == ' ' {
			return '-'
		}
		if r == '(' || r == ')' {
			return -1
		}
		return unicode.ToLower(r)
	}, s.Name))
	text = fmt.Sprintf("; %s (from the Computer Intro! manual)\n; %s\n; Step  Hex    Assembler\n%s",
		s.Name, s.Usage, strings.TrimLeft(s.Listing, "\n"))
	return name, text
}

// TestExportSamples writes the manual's programs as listings when
// ODYSSEY9_EXPORT names a directory. go test runs in machine/, so to
// regenerate the examples: ODYSSEY9_EXPORT=../examples go test ./machine
func TestExportSamples(t *testing.T) {
	dir := os.Getenv("ODYSSEY9_EXPORT")
	if dir == "" {
		t.Skip("ODYSSEY9_EXPORT not set")
	}
	for i, s := range Samples {
		name, text := sampleFile(i, s)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestExamplesMatchSamples checks that examples/ holds exactly the listings
// TestExportSamples would write.
func TestExamplesMatchSamples(t *testing.T) {
	files, err := filepath.Glob("../examples/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(Samples) {
		t.Errorf("examples/ has %d listings, want %d", len(files), len(Samples))
	}
	for i, s := range Samples {
		name, want := sampleFile(i, s)
		got, err := os.ReadFile(filepath.Join("..", "examples", name))
		if err != nil {
			t.Error(err)
			continue
		}
		if string(got) != want {
			t.Errorf("examples/%s differs from Samples[%d]; regenerate it with TestExportSamples", name, i)
		}
	}
}

// Flash Card's problem blinks until it is solved.
func TestFlashCardBlinks(t *testing.T) {
	m := New()
	m.Press(Key2)
	shown := func() bool { return strings.TrimSpace(screenText(m)) != "" }
	for i, want := range []bool{false, true, false} {
		for range 40 { // 2/3 s: longer than either blink
			m.Update(time.Second / 60)
			if shown() == want {
				break
			}
		}
		if shown() != want {
			t.Fatalf("blink %d: problem shown %v, want %v", i, shown(), want)
		}
	}
}

// Between the Sheets starts the next round after showing the answer.
func TestBetweenSheetsNextRound(t *testing.T) {
	m := New()
	m.Press(Key4)
	g := m.game.(*betweenSheets)
	g.lo, g.hi, g.secret = 3, 7, 5
	m.Press(KeyYes)
	for range 150 { // 2.5 s
		m.Update(time.Second / 60)
	}
	if !g.waiting {
		t.Error("no new round 2.5 s after the answer")
	}
}

// Opcodes the manual doesn't list do nothing.
func TestUndefinedOpcodeDoesNothing(t *testing.T) {
	m := New()
	m.LoadProgram(&[Steps]byte{0x06, 0x03, 0xFF}) // ?, INC, HLT
	m.Press(KeyE)
	m.Update(time.Second)
	if m.A != 0x01 || m.Status() != "HALT" {
		t.Errorf("A=%02X, status %q; want 01 and HALT", m.A, m.Status())
	}
}

// The ERROR note goes after a moment.
func TestErrorNoteClears(t *testing.T) {
	m := New()
	for _, k := range []Key{KeyReset, KeyP, KeyA, KeyI, KeyX, KeyEnter} {
		m.Press(k)
	}
	if !strings.Contains(m.Status(), "ERROR") {
		t.Fatalf("status %q after a bad statement, want ERROR", m.Status())
	}
	m.Update(2 * time.Second)
	if strings.Contains(m.Status(), "ERROR") {
		t.Errorf("status %q 2 s later", m.Status())
	}
}

// Keys read by INP have the codes in the manual's Key Codes table.
func TestKeyCodes(t *testing.T) {
	for k, want := range map[Key]byte{KeyYes: 0x2C, KeyNo: 0x2D, KeyClear: 0x2E, KeyEnter: 0x2F, KeyQuestion: 0x0D, Key7: 0x07, KeyA: 0x20} {
		if got := k.Code(); got != want {
			t.Errorf("%q: code %02X, want %02X", k.Rune(), got, want)
		}
	}
}

// Time spent waiting at INP isn't made up for once the key comes, even at
// SLOW, where a stall isn't otherwise limited.
func TestNoBurstAfterWaitingForKey(t *testing.T) {
	m := assembled(t, nil, "INP.0", "INC", "GTO.01")
	m.SetSpeed(SpeedSlow)
	m.Press(KeyE)
	m.Update(time.Second / 60)
	m.Update(time.Minute)
	m.Press(Key5)
	m.Update(time.Second / 60)
	if m.A > 0x01 {
		t.Errorf("A=%02X: the program ran ahead after waiting a minute for a key", m.A)
	}
}
