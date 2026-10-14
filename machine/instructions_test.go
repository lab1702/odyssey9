package machine

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

// assembled returns a machine in Command mode holding the statements in
// consecutive steps from 00, with any extra steps given in at.
func assembled(t *testing.T, at map[int]string, src ...string) *Machine {
	t.Helper()
	var mem [Steps]byte
	put := func(step int, s string) int {
		code, err := Assemble(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		return step + copy(mem[step:], code)
	}
	step := 0
	for _, s := range src {
		step = put(step, s)
	}
	for step, s := range at {
		put(step, s)
	}
	m := New()
	m.LoadProgram(&mem)
	return m
}

// runToHalt executes the program from step 00 until it stops.
func runToHalt(t *testing.T, m *Machine) {
	t.Helper()
	m.Press(KeyE)
	for range 10000 {
		if m.mode != ModeExecute {
			return
		}
		m.Step()
	}
	t.Fatalf("program never stopped (PC=%02d)", m.PC)
}

// press presses each key in turn.
func press(m *Machine, keys ...Key) {
	for _, k := range keys {
		m.Press(k)
	}
}

// keysFor returns the keys that type s.
func keysFor(s string) []Key {
	var keys []Key
	for _, r := range s {
		switch r {
		case '.':
			keys = append(keys, KeyPeriod)
		default:
			keys = append(keys, KeyForLetter(r))
		}
	}
	return keys
}

func TestBranches(t *testing.T) {
	cases := []struct {
		name  string
		src   []string // set up A and registers, then branch to step 20
		taken bool
	}{
		{"BDB after 00 - 1 = 99", []string{"CLR", "DEC", "BDB.20"}, true},
		{"BDB with 90", []string{"LDV.0.90", "LDA.0", "BDB.20"}, true},
		{"BDB with 89", []string{"LDV.0.89", "LDA.0", "BDB.20"}, false},
		{"BDC after 5 + 7 = 12", []string{"LDV.0.05", "LDA.0", "LDV.1.07", "ADD.1", "BDC.20"}, true},
		{"BDC with 09", []string{"LDV.0.09", "LDA.0", "BDC.20"}, false},
		{"BDC after 99 + 1 = 00", []string{"LDV.0.99", "LDA.0", "INC", "BDC.20"}, false},
		{"BGT register above A", []string{"LDV.0.05", "LDA.0", "LDV.1.06", "BGT.1.20"}, true},
		{"BGT register equal to A", []string{"LDV.0.05", "LDA.0", "BGT.0.20"}, false},
		{"BGT register below A", []string{"LDV.0.05", "LDA.0", "LDV.1.04", "BGT.1.20"}, false},
		{"BLS register below A", []string{"LDV.0.05", "LDA.0", "LDV.1.04", "BLS.1.20"}, true},
		{"BLS register equal to A", []string{"LDV.0.05", "LDA.0", "BLS.0.20"}, false},
		{"BLS register above A", []string{"LDV.0.05", "LDA.0", "LDV.1.06", "BLS.1.20"}, false},
	}
	for _, c := range cases {
		src := append(c.src, "LDV.D.01", "HLT")
		m := assembled(t, map[int]string{20: "LDV.D.02", 22: "HLT"}, src...)
		runToHalt(t, m)
		if taken := m.R[0xD] == 0x02; taken != c.taken {
			t.Errorf("%s: branch taken = %v, want %v", c.name, taken, c.taken)
		}
	}
}

func TestOutputPositionWraps(t *testing.T) {
	m := assembled(t, nil, "LDV.0.01", "LDV.B.0A", "OUT.0", "OUT.0", "HLT")
	runToHalt(t, m)
	if m.Screen[10] != 0x01 || m.Screen[0] != 0x01 || m.R[RegB] != 0x01 {
		t.Errorf("OUT at 0A then wrap: screen % X, B=%02X", m.Screen, m.R[RegB])
	}

	m = assembled(t, nil, "LDV.0.01", "LDA.0", "LDV.B.0F", "OTA", "HLT")
	runToHalt(t, m)
	if m.Screen[0] != 0x01 || m.R[RegB] != 0x01 {
		t.Errorf("OTA with B=0F: screen % X, B=%02X", m.Screen, m.R[RegB])
	}
}

func TestMOV(t *testing.T) {
	m := assembled(t, nil, "LDV.C.99", "MOV", "HLT")
	m.Mem[99] = 0x42
	runToHalt(t, m)
	if m.A != 0x42 || m.R[RegC] != 0x00 {
		t.Errorf("MOV with C=99: A=%02X C=%02X, want 42 00", m.A, m.R[RegC])
	}

	// A digit above 9 counts as 10 or more, as in the branch targets.
	m = assembled(t, nil, "LDV.C.0A", "MOV", "HLT")
	m.Mem[10] = 0x37
	runToHalt(t, m)
	if m.A != 0x37 || m.R[RegC] != 0x11 {
		t.Errorf("MOV with C=0A: A=%02X C=%02X, want 37 11", m.A, m.R[RegC])
	}
}

func TestHexInputEditing(t *testing.T) {
	m := New()
	press(m, KeyReset, KeyP, KeyM, KeyI, Key1, Key2, Key3)
	if got := m.Status(); got != "STEP 00 23" {
		t.Errorf("after 1 2 3: %q, want only the last two digits", got)
	}
	m.Backspace()
	if got := m.Status(); got != "STEP 00 2" {
		t.Errorf("after Backspace: %q", got)
	}
	m.Press(KeyEnter)
	if m.Mem[0] != 0 || m.CurrentSound() != SoundError || m.Status() != "STEP 00 2" {
		t.Errorf("ENTER with one digit: step 00 = %02X, sound %v, status %q", m.Mem[0], m.CurrentSound(), m.Status())
	}
	m.Backspace()
	m.Backspace() // nothing left to erase
	press(m, KeyA, Key5, KeyEnter)
	if m.Mem[0] != 0xA5 || m.Status() != "STEP 01 " {
		t.Errorf("after A 5 ENTER: step 00 = %02X, status %q", m.Mem[0], m.Status())
	}
}

func TestAssemblerInputLimit(t *testing.T) {
	m := New()
	press(m, KeyReset, KeyP, KeyA, KeyI)
	press(m, keysFor("LDV.0.12XY")...)
	if m.CurrentSound() == SoundError {
		t.Fatal("ten characters were refused")
	}
	m.Press(KeyZ)
	if m.CurrentSound() != SoundError || m.Status() != "STEP 00 LDV.0.12XY" {
		t.Errorf("11th character: sound %v, status %q", m.CurrentSound(), m.Status())
	}
	m.Backspace()
	m.Backspace()
	m.Press(KeyEnter)
	if m.Mem[0] != 0x60 || m.Mem[1] != 0x12 || m.Status() != "STEP 02 " {
		t.Errorf("after erasing XY: % X, status %q", m.Mem[:2], m.Status())
	}
}

func TestStatusTexts(t *testing.T) {
	m := New()
	for _, c := range []struct {
		keys []Key
		want string
	}{
		{[]Key{KeyReset}, "COMMAND"},
		{[]Key{KeyP}, "PROGRAM"},
		{[]Key{KeyA}, "ASSEMBLER"},
		{[]Key{KeyClear, KeyP, KeyM}, "HEX INPUT"},
		{[]Key{KeyR}, "ROLL 00  00"},
		{[]Key{KeyClear, KeyClear}, "COMMAND"},
	} {
		press(m, c.keys...)
		if got := m.Status(); got != c.want {
			t.Errorf("after %v: %q, want %q", c.keys, got, c.want)
		}
	}

	m = assembled(t, nil, "INP.0", "HLT")
	m.Press(KeyE)
	m.Update(time.Second / 60)
	if got := m.Status(); got != "?" {
		t.Errorf("waiting for a key: %q, want %q", got, "?")
	}
}

func TestResetAfterHalt(t *testing.T) {
	m := assembled(t, nil, "LDV.0.01", "HLT", "LDV.0.02", "HLT")
	runToHalt(t, m)
	if got := m.Status(); got != "HALT" {
		t.Fatalf("after HLT: %q", got)
	}
	m.Press(KeyReset)
	if got := m.Status(); got != "COMMAND" {
		t.Errorf("RESET after HLT: %q, want COMMAND", got)
	}
	m.Press(KeyC)
	for range 100 {
		if m.mode != ModeExecute {
			break
		}
		m.Step()
	}
	if m.R[0] != 0x02 {
		t.Errorf("C after RESET didn't continue past the HLT: R0=%02X", m.R[0])
	}
}

// TestStepsPerUpdate checks how many steps run in a given time: INC, GTO.00
// adds one to A every two steps.
func TestStepsPerUpdate(t *testing.T) {
	for _, c := range []struct {
		name  string
		speed int
		dt    time.Duration
		want  byte
	}{
		{"SLOW for a second", SpeedSlow, time.Second, 0x02},
		{"NORMAL for 50 ms", SpeedNormal, 50 * time.Millisecond, 0x25},
		{"NORMAL after a stall", SpeedNormal, time.Second, 0x50}, // at most 0.1 s is caught up
		{"FAST for 0.5 ms", SpeedFast, 500 * time.Microsecond, 0x50},
	} {
		m := assembled(t, nil, "INC", "GTO.00")
		m.SetSpeed(c.speed)
		m.Press(KeyE)
		m.Update(c.dt)
		if m.A != c.want {
			t.Errorf("%s: A=%02X, want %02X", c.name, m.A, c.want)
		}
	}
}

// When SIG's second ends part-way through an update, the rest of that
// update's time is used to run the program.
func TestTimeLeftAfterSig(t *testing.T) {
	m := assembled(t, nil, "SIG", "INC", "GTO.01")
	m.Press(KeyE)
	m.Update(time.Millisecond) // runs the SIG
	if m.blockLeft != time.Second {
		t.Fatalf("SIG didn't pause the program: %v", m.blockLeft)
	}
	m.Update(time.Second + 10*time.Millisecond)
	if m.A != 0x05 {
		t.Errorf("10 ms after SIG: A=%02X, want 05", m.A)
	}
}

func TestRandomProgramsRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := range 2000 {
		var mem [Steps]byte
		for j := range mem {
			switch rng.Intn(3) {
			case 0: // leave a run of empty steps
			case 1:
				mem[j] = byte(rng.Intn(256))
			default:
				mem[j] = opTable[rng.Intn(len(opTable))].op | byte(rng.Intn(16))
			}
		}
		var sb strings.Builder
		FormatProgram(&sb, "t", &mem)
		p, err := ParseProgram(strings.NewReader(sb.String()))
		if err != nil || *p != mem {
			t.Fatalf("program %d didn't round-trip (%v):\n%s", i, err, sb.String())
		}
	}
}
