package machine

import (
	"bytes"
	"fmt"
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
	for i := 0; i < 200000; i++ {
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
	for _, bad := range []string{"LDV.B", "XYZ", "OUT.10", "GTO.123"} {
		if _, err := Assemble(bad); err == nil {
			t.Errorf("%s assembled without error", bad)
		}
	}
	if code, _ := Assemble("BVNE.4.24"); !bytes.Equal(code, []byte{0x24, 0x24}) {
		t.Errorf("BVNE alias: % X", code)
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
}

func TestCreepyCrawlerII(t *testing.T) {
	m := New()
	m.LoadProgram(Samples[1].Program())
	m.Press(KeyE)
	for i := 0; i < 500; i++ {
		m.blockLeft = 0
		m.Step()
	}
	symbols := map[byte]bool{0x32: true, 0x33: true, 0x3A: true, 0x34: true, 0x35: true,
		0x37: true, 0x3D: true, 0x3E: true, 0x36: true, 0x3C: true, blank: true}
	for _, c := range m.Screen {
		if !symbols[c] {
			t.Fatalf("unexpected symbol %02X on screen", c)
		}
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
}

// TestExportSamples writes the manual's programs as listings when
// ODYSSEY9_EXPORT names a directory: ODYSSEY9_EXPORT=examples go test ./machine
func TestExportSamples(t *testing.T) {
	dir := os.Getenv("ODYSSEY9_EXPORT")
	if dir == "" {
		t.Skip("ODYSSEY9_EXPORT not set")
	}
	for i, s := range Samples {
		name := fmt.Sprintf("%02d-%s.txt", i+1, strings.Map(func(r rune) rune {
			if r == ' ' {
				return '-'
			}
			if r == '(' || r == ')' {
				return -1
			}
			return unicode.ToLower(r)
		}, s.Name))
		text := fmt.Sprintf("; %s (from the Computer Intro! manual)\n; %s\n; Step  Hex    Assembler\n%s",
			s.Name, s.Usage, strings.TrimLeft(s.Listing, "\n"))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
