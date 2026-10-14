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
		"remark gap":     "00 60 05  BE QUIET\n02 61 07\t0A\n04 FF\n",
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
	}
}
