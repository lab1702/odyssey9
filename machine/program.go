package machine

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// ParseProgram reads a program listing. Each line starts with a decimal
// step number followed by one or more two-digit hex bytes separated by
// single spaces. The bytes end at the first word that isn't one or at a gap
// of two or more spaces or a tab; anything after that (an assembler
// mnemonic, a remark) is ignored, as is any text after ';' or '#'. But if
// what follows the gap is nothing but more hex bytes, the line is refused
// rather than loaded without them. Lines may end in LF, CR LF or CR, and
// any other Unicode space counts as a space. The text is UTF-8, or UTF-16 if it starts with a byte order mark,
// as Windows tools may save it. Each step may be given only once, and a
// line may not run past step 99. This accepts the listing format used in
// the manual:
//
//	04   6B 00  LDV.B.00   Set output position to 00
func ParseProgram(r io.Reader) (*[Steps]byte, error) {
	var mem [Steps]byte
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text, err := decodeText(data)
	if err != nil {
		return nil, err
	}
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text)
	var setOn [Steps]int // the line that gave each step, or 0
	lineNo, count := 0, 0
	for line := range strings.SplitSeq(text, "\n") {
		lineNo++
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\uFEFF") // UTF-8 byte order mark
		}
		if i := strings.IndexAny(line, ";#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(strings.Map(plainSpace, line))
		if line == "" {
			continue
		}
		first, rest, _ := strings.Cut(strings.ReplaceAll(line, "\t", "  "), " ")
		step, err := strconv.Atoi(first)
		if err != nil || len(first) > 2 || strings.Trim(first, "0123456789") != "" {
			return nil, fmt.Errorf("line %d: %q is not a step number 00-99", lineNo, first)
		}
		// A wider gap separates the bytes from a remark, so a remark that
		// starts with a hex-like word ("BE QUIET") isn't read as code.
		rest, remark, _ := strings.Cut(strings.TrimLeft(rest, " "), "  ")
		n, words := 0, 0
		for f := range strings.SplitSeq(rest, " ") {
			words++
			v, ok := hexByte(f)
			if !ok {
				break
			}
			at := step + n
			if at >= Steps {
				return nil, fmt.Errorf("line %d: runs past step %02d", lineNo, Steps-1)
			}
			if setOn[at] != 0 {
				return nil, fmt.Errorf("line %d: step %02d was already given on line %d", lineNo, at, setOn[at])
			}
			mem[at], setOn[at] = v, lineNo
			n++
		}
		if n == 0 {
			return nil, fmt.Errorf("line %d: no hex code after step %02d", lineNo, step)
		}
		// Bytes set apart by a tab or a wider gap, as in a listing with a
		// column for each byte, would be dropped as a remark without a word.
		if col, _, _ := strings.Cut(strings.TrimLeft(remark, " "), "  "); n == words && allHexBytes(col) {
			return nil, fmt.Errorf("line %d: %q after a tab or a wide gap looks like more code: put one space between bytes", lineNo, col)
		}
		count += n
	}
	if count == 0 {
		return nil, fmt.Errorf("no program steps found")
	}
	return &mem, nil
}

// hexByte returns the value of a two-digit hex byte such as "6B".
func hexByte(s string) (byte, bool) {
	if len(s) != 2 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 8)
	return byte(v), err == nil
}

// allHexBytes reports whether s is one or more two-digit hex bytes with a
// space between them, and nothing else.
func allHexBytes(s string) bool {
	if s == "" {
		return false
	}
	for f := range strings.SplitSeq(s, " ") {
		if _, ok := hexByte(f); !ok {
			return false
		}
	}
	return true
}

// plainSpace turns any Unicode space other than a tab, such as a no-break
// space pasted from a web page, into a plain space.
func plainSpace(r rune) rune {
	if r != '\t' && unicode.IsSpace(r) {
		return ' '
	}
	return r
}

// FormatProgram writes program memory as a listing that ParseProgram can
// read back. The steps the program can reach (see CodeSteps) are written
// as instructions, and the others as data, up to dataPerLine bytes to a
// line, as the manual lists a message, with its text if it is one. Empty
// (00) steps of data and trailing empty steps are omitted, but step 00 is
// always written so that an empty program still loads.
func FormatProgram(w io.Writer, title string, mem *[Steps]byte) error {
	last := 0
	for i := Steps - 1; i > 0; i-- {
		if mem[i] != 0 {
			last = i
			break
		}
	}
	code := CodeSteps(mem)
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "; %s\n; Odyssey2 Computer Intro! program listing\n; Step  Hex    Assembler\n", title)
	for step := 0; step <= last; {
		if code[step] {
			text, n := Disassemble(mem, step)
			fmt.Fprintf(bw, "%02d    %-6s %s\n", step, HexBytes(mem[step:step+n]), text)
			step += n
			continue
		}
		data := dataLine(mem, &code, step, last)
		if len(data) > 1 || data[0] != 0 {
			fmt.Fprintf(bw, "%02d    %s", step, HexBytes(data))
			if text := dataText(data); text != "" {
				// In quotes, so that no text reads back as more bytes.
				fmt.Fprintf(bw, "  %q", text)
			}
			fmt.Fprintln(bw)
		}
		step += len(data)
	}
	return bw.Flush()
}

// HexBytes writes bytes as a listing does: two hex digits each, with a
// space between.
func HexBytes(b []byte) string {
	return fmt.Sprintf("% X", b)
}

// dataPerLine is the most data bytes FormatProgram writes on a line.
const dataPerLine = 12

// dataLine returns the data that goes on one line of a listing from step
// on: the steps up to last that hold no instruction, ending after a 00,
// which ends a message.
func dataLine(mem *[Steps]byte, code *[Steps]bool, step, last int) []byte {
	n := 0
	for step+n <= last && !code[step+n] && n < dataPerLine {
		n++
		if mem[step+n-1] == 0 {
			break
		}
	}
	return mem[step : step+n]
}

// dataText returns the text that data shows on the screen, without the 00
// that ends it, or "" if it isn't text: it has a symbol with no ASCII
// character, or no letter at all, as a table of numbers has.
func dataText(data []byte) string {
	if n := len(data); n > 1 && data[n-1] == 0 {
		data = data[:n-1]
	}
	var sb strings.Builder
	letter := false
	for _, b := range data {
		r, ok := CodeRune(b)
		if !ok {
			return ""
		}
		letter = letter || unicode.IsLetter(r)
		sb.WriteRune(r)
	}
	if !letter {
		return ""
	}
	return sb.String()
}

// decodeText returns data as a string, decoding it from UTF-16 if it starts
// with a UTF-16 byte order mark, which it drops. A UTF-16 file with an odd
// number of bytes has lost the end of its last character, and perhaps more.
func decodeText(data []byte) (string, error) {
	var get func(b []byte) uint16
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		get = func(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		get = func(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }
	default:
		return string(data), nil
	}
	if len(data)%2 != 0 {
		return "", fmt.Errorf("this UTF-16 file ends part-way through a character")
	}
	u := make([]uint16, 0, len(data)/2)
	for i := 2; i+1 < len(data); i += 2 {
		u = append(u, get(data[i:]))
	}
	return string(utf16.Decode(u)), nil
}
