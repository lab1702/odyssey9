package machine

import (
	"strings"
	"testing"
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
