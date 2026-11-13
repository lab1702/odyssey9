package machine

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestParseProgramSpacing(t *testing.T) {
	want := []byte{0x60, 0x05, 0x61, 0x07, 0xFF}
	for name, src := range map[string]string{
		"LF":             "00 60 05\n02 61 07\n04 FF\n",
		"CR LF":          "00 60 05\r\n02 61 07\r\n04 FF\r\n",
		"CR":             "00 60 05\r02 61 07\r04 FF\r",
		"no-break space": "00 60 05\n02 61 07 \n 04 FF\n",
		"remark gap":     "00 60 05  BE QUIET\n02 61 07\t0A IS TEN\n04 FF\n",
	} {
		p, err := ParseProgram(strings.NewReader(src))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := p[:len(want)+1]; string(got) != string(append(want, 0)) {
			t.Errorf("%s: loaded % X, want % X 00", name, got, want)
		}
	}
}

// Bytes set apart from the first by a tab or a wider gap, as in a listing
// with a column for each byte, are refused rather than dropped as a remark.
func TestParseProgramByteColumns(t *testing.T) {
	for _, src := range []string{
		"05\t6B\t00\n",
		"05\t6B\t00\tLDV.B.00\n",
		"05  6B  00\n",
		"05 6B   00 01  remark\n",
		"30 1D 12\t0E 0E 17 00\n",
	} {
		if _, err := ParseProgram(strings.NewReader(src)); err == nil {
			t.Errorf("%q loaded without its last bytes", src)
		}
	}
	// A remark is only taken for bytes if it is nothing else.
	for _, src := range []string{
		"05 6B 00  0 = first position\n",
		"05\t6B 00\tLDV.B.00\tposition 00\n",
		"05 6B 00  BE QUIET  00\n",
		"05 6B 00 LDV.B.00  00\n",
	} {
		p, err := ParseProgram(strings.NewReader(src))
		if err != nil || p[5] != 0x6B || p[6] != 0 || p[7] != 0 {
			t.Errorf("%q: %v", src, err)
		}
	}
}

func TestParseProgramStepNumbers(t *testing.T) {
	for _, step := range []string{"+5", "-0", "+0", "-1", "100", "5a", "٣"} {
		if _, err := ParseProgram(strings.NewReader(step + " 60 05\n")); err == nil {
			t.Errorf("step %q accepted", step)
		}
	}
	for _, step := range []string{"5", "05", "99"} {
		if _, err := ParseProgram(strings.NewReader(step + " 60\n")); err != nil {
			t.Errorf("step %q: %v", step, err)
		}
	}
}

func TestParseProgramOverlaps(t *testing.T) {
	for _, src := range []string{
		"99 01 02\n", // past step 99
		"00 60 90\n98 12 05 77\n",
		"00 60 05\n01 61\n", // step 01 given twice
		"04 C0\n00 60 05\n04 C1\n",
	} {
		if _, err := ParseProgram(strings.NewReader(src)); err == nil {
			t.Errorf("%q accepted", src)
		}
	}
	// Lines may still come in any order.
	p, err := ParseProgram(strings.NewReader("02 C0\n00 60 05\n"))
	if err != nil || p[0] != 0x60 || p[2] != 0xC0 {
		t.Errorf("steps out of order: %v", err)
	}
}

// Listings saved as UTF-16, as Windows PowerShell's > and Notepad's
// "Unicode" do, load like UTF-8 ones.
func TestParseProgramUTF16(t *testing.T) {
	src := "\uFEFF00 60 05  LDV.0.05\r\n02 FF\r\n"
	for name, big := range map[string]bool{"little-endian": false, "big-endian": true} {
		var b []byte
		for _, u := range utf16.Encode([]rune(src)) {
			if big {
				b = append(b, byte(u>>8), byte(u))
			} else {
				b = append(b, byte(u), byte(u>>8))
			}
		}
		p, err := ParseProgram(bytes.NewReader(b))
		if err != nil || p[0] != 0x60 || p[1] != 0x05 || p[2] != 0xFF {
			t.Errorf("%s: %v", name, err)
		}
		if _, err := ParseProgram(bytes.NewReader(b[:len(b)-3])); err == nil {
			t.Errorf("%s: a file cut part-way through a character was accepted", name)
		}
	}
}

// A listing shows the steps a program can reach as instructions and the
// rest as data, with the text of a message, and loads back the same.
func TestFormatProgramData(t *testing.T) {
	for _, s := range Samples {
		var buf bytes.Buffer
		if err := FormatProgram(&buf, s.Name, s.Program()); err != nil {
			t.Fatal(err)
		}
		text := buf.String()
		if p, err := ParseProgram(&buf); err != nil || *p != *s.Program() {
			t.Errorf("%s: listing doesn't load back the same: %v", s.Name, err)
		}
		if strings.Contains(text, "??") {
			t.Errorf("%s: data listed as instructions:\n%s", s.Name, text)
		}
		for _, want := range map[string][]string{
			"Message": {
				"28    12 24  GTO.24\n",
				"30    1D 12 0E 0E 17 00  \"HELLO\"\n",
				"59    2D 12 11 0C 1B 17 13 0C 07 08 00  \"NEW FOR 78\"\n",
				"91    30 36 48 59 70 81\n",
			},
			"Creepy Crawler II": {"13    32 33 3A 34 35 37 3D 3E 36 3C\n"},
		}[s.Name] {
			if !strings.Contains(text, want) {
				t.Errorf("%s: listing lacks %q:\n%s", s.Name, want, text)
			}
		}
	}
}

// Empty steps between a program and its data aren't listed, and text that
// could be read back as bytes is in quotes.
func TestFormatProgramDataGaps(t *testing.T) {
	mem := [Steps]byte{0: 0x12, 1: 0x00} // GTO.00
	copy(mem[40:], []byte{0x25, 0x12, 0x00, 0x00, 0x07, 0x00, 0x20})
	var buf bytes.Buffer
	FormatProgram(&buf, "t", &mem)
	_, lines, _ := strings.Cut(buf.String(), "Assembler\n")
	if want := "00    12 00  GTO.00\n40    25 12 00  \"BE\"\n44    07 00\n46    20  \"A\"\n"; lines != want {
		t.Errorf("listing:\n%swant:\n%s", lines, want)
	}
	if p, err := ParseProgram(&buf); err != nil || *p != mem {
		t.Errorf("listing doesn't load back the same: %v", err)
	}
}

func TestCodeSteps(t *testing.T) {
	for name, c := range map[string]struct {
		mem  [Steps]byte
		code []int // the steps that start an instruction, if not all
	}{
		"empty program runs through every step": {},
		"GTO ends the code":                     {[Steps]byte{0x03, 0x12, 0x00, 0x1D}, []int{0, 1}},
		"a branch goes both ways":               {[Steps]byte{0x13, 0x05, 0x12, 0x00, 0x1D, 0x12, 0x05}, []int{0, 2, 5}},
		"LDV's operand is no step":              {[Steps]byte{0x60, 0x50, 0x12, 0x00}, []int{0, 2}},
		"RET ends a subroutine":                 {[Steps]byte{0x14, 0x10, 0x12, 0x00, 10: 0x03, 0x07, 0x1D}, []int{0, 2, 10, 11}},
		"HLT goes on, as C does":                {[Steps]byte{0xFF, 0x12, 0x01}, []int{0, 1}},
		"a target in hex wraps as it runs":      {[Steps]byte{0x12, 0xA5, 5: 0x12, 0x05}, []int{0, 5}},
		"step 99 takes its operand from 00":     {[Steps]byte{0x00, 0x12, 0x99, 99: 0x12}, []int{0, 1, 99}},
	} {
		got := CodeSteps(&c.mem)
		var want [Steps]bool
		for _, s := range c.code {
			want[s] = true
		}
		if c.code == nil {
			for s := range want {
				want[s] = true
			}
		}
		if got != want {
			var steps []int
			for s, is := range got {
				if is {
					steps = append(steps, s)
				}
			}
			t.Errorf("%s: code at steps %v, want %v", name, steps, c.code)
		}
	}
}
