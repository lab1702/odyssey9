package machine

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ParseProgram reads a program listing. Each line starts with a decimal
// step number followed by one or more two-digit hex bytes separated by
// single spaces. The bytes end at the first word that isn't one or at a gap
// of two or more spaces or a tab; anything after that (an assembler
// mnemonic, a remark) is ignored, as is any text after ';' or '#'. This
// accepts the listing format used in the manual:
//
//	04   6B 00  LDV.B.00   Set output position to 00
func ParseProgram(r io.Reader) (*[Steps]byte, error) {
	var mem [Steps]byte
	sc := bufio.NewScanner(r)
	lineNo, count := 0, 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\uFEFF") // UTF-8 byte order mark
		}
		if i := strings.IndexAny(line, ";#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		first, rest, _ := strings.Cut(strings.ReplaceAll(line, "\t", "  "), " ")
		step, err := strconv.Atoi(first)
		if err != nil || step < 0 || step >= Steps || len(first) > 2 {
			return nil, fmt.Errorf("line %d: %q is not a step number 00-99", lineNo, first)
		}
		// A wider gap separates the bytes from a remark, so a remark that
		// starts with a hex-like word ("BE QUIET") isn't read as code.
		rest, _, _ = strings.Cut(strings.TrimLeft(rest, " "), "  ")
		n := 0
		for _, f := range strings.Split(rest, " ") {
			if len(f) != 2 {
				break
			}
			v, err := strconv.ParseUint(f, 16, 8)
			if err != nil {
				break
			}
			mem[(step+n)%Steps] = byte(v)
			n++
		}
		if n == 0 {
			return nil, fmt.Errorf("line %d: no hex code after step %02d", lineNo, step)
		}
		count += n
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("no program steps found")
	}
	return &mem, nil
}

// FormatProgram writes program memory as a listing that ParseProgram can
// read back. Trailing empty (00) steps are omitted, but step 00 is always
// written so that an empty program still loads.
func FormatProgram(w io.Writer, title string, mem *[Steps]byte) error {
	last := 0
	for i := Steps - 1; i > 0; i-- {
		if mem[i] != 0 {
			last = i
			break
		}
	}
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "; %s\n; Odyssey2 Computer Intro! program listing\n; Step  Hex    Assembler\n", title)
	for step := 0; step <= last; {
		text, n := Disassemble(mem, step)
		hex := fmt.Sprintf("%02X", mem[step])
		if n == 2 {
			hex += fmt.Sprintf(" %02X", mem[step+1])
		}
		fmt.Fprintf(bw, "%02d    %-6s %s\n", step, hex, text)
		step += n
	}
	return bw.Flush()
}
