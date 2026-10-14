package machine

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Operand forms used by the instruction set (manual pages 100-103).
type form int

const (
	formNone form = iota // XX            e.g. NOP
	formR                // XR            e.g. OUT.R
	formNN               // XX NN         e.g. GTO.NN
	formRNN              // XR NN         e.g. LDV.R.NN
)

type opInfo struct {
	mnemonic string
	op       byte
	form     form
}

var opTable = []opInfo{
	{"NOP", 0x00, formNone},
	{"CLR", 0x01, formNone},
	{"DEC", 0x02, formNone},
	{"INC", 0x03, formNone},
	{"INA", 0x04, formNone},
	{"SIG", 0x05, formNone},
	{"RET", 0x07, formNone},
	{"RND", 0x08, formNone},
	{"MOV", 0x09, formNone},
	{"OTA", 0x0B, formNone},
	{"HLT", 0xFF, formNone},
	{"BDB", 0x10, formNN},
	{"BDC", 0x11, formNN},
	{"GTO", 0x12, formNN},
	{"BRZ", 0x13, formNN},
	{"GTS", 0x14, formNN},
	{"BNE", 0x20, formRNN},
	{"BEQ", 0x30, formRNN},
	{"BGT", 0x40, formRNN},
	{"BLS", 0x50, formRNN},
	{"LDV", 0x60, formRNN},
	{"INP", 0x70, formR},
	{"PAK", 0x80, formR},
	{"LDA", 0x90, formR},
	{"STO", 0xA0, formR},
	{"UNP", 0xB0, formR},
	{"OUT", 0xC0, formR},
	{"SUB", 0xD0, formR},
	{"ADD", 0xE0, formR},
}

var opByMnemonic = func() map[string]opInfo {
	m := map[string]opInfo{}
	for _, o := range opTable {
		m[o.mnemonic] = o
	}
	m["BVNE"] = m["BNE"] // spelling used in the manual's instruction chart
	return m
}()

// decode returns the instruction descriptor for an opcode byte.
func decode(op byte) (opInfo, bool) {
	for _, o := range opTable {
		switch o.form {
		case formNone, formNN:
			if op == o.op {
				return o, true
			}
		case formR, formRNN:
			if op&0xF0 == o.op {
				return o, true
			}
		}
	}
	return opInfo{}, false
}

// InstrLen is the number of bytes (program steps) an opcode occupies.
func InstrLen(op byte) int {
	o, ok := decode(op)
	if ok && (o.form == formNN || o.form == formRNN) {
		return 2
	}
	return 1
}

// Disassemble renders the instruction at step in assembler notation and
// returns its length in steps. Unknown opcodes render as "??", followed
// by the letter when the byte is a letter's character code (message data).
func Disassemble(mem *[Steps]byte, step int) (string, int) {
	op := mem[step%Steps]
	o, ok := decode(op)
	if !ok {
		if r, ok := codeLetters[op]; ok {
			return fmt.Sprintf("?? '%c'", r), 1
		}
		return "??", 1
	}
	nn := mem[(step+1)%Steps]
	r := op & 0x0F
	switch o.form {
	case formR:
		return fmt.Sprintf("%s.%X", o.mnemonic, r), 1
	case formNN:
		return fmt.Sprintf("%s.%02X", o.mnemonic, nn), 2
	case formRNN:
		return fmt.Sprintf("%s.%X.%02X", o.mnemonic, r, nn), 2
	}
	return o.mnemonic, 1
}

// Assemble converts one assembler statement such as "LDV.B.00" into
// machine code.
func Assemble(src string) ([]byte, error) {
	src = strings.ToUpper(strings.TrimSpace(src))
	if src == "" {
		return nil, errors.New("empty statement")
	}
	parts := strings.Split(src, ".")
	o, ok := opByMnemonic[parts[0]]
	if !ok {
		return nil, fmt.Errorf("unknown instruction %q", parts[0])
	}
	want := map[form]int{formNone: 1, formR: 2, formNN: 2, formRNN: 3}[o.form]
	if len(parts) != want {
		return nil, fmt.Errorf("%s: wrong number of operands", o.mnemonic)
	}
	reg := func(s string) (byte, error) {
		if len(s) != 1 {
			return 0, fmt.Errorf("register must be 0-F, got %q", s)
		}
		v, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			return 0, fmt.Errorf("register must be 0-F, got %q", s)
		}
		return byte(v), nil
	}
	val := func(s string) (byte, error) {
		if len(s) < 1 || len(s) > 2 {
			return 0, fmt.Errorf("value must be two hex digits, got %q", s)
		}
		v, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			return 0, fmt.Errorf("value must be two hex digits, got %q", s)
		}
		return byte(v), nil
	}
	// Branch and subroutine targets are step numbers, written in decimal
	// (GTO.15 is coded 12 15). LDV's operand is a value and may be any hex.
	step := func(s string) (byte, error) {
		n, err := val(s)
		if err == nil && (n>>4 > 9 || n&0x0F > 9) {
			err = fmt.Errorf("step must be 00-99, got %q", s)
		}
		return n, err
	}
	switch o.form {
	case formR:
		r, err := reg(parts[1])
		if err != nil {
			return nil, err
		}
		return []byte{o.op | r}, nil
	case formNN:
		n, err := step(parts[1])
		if err != nil {
			return nil, err
		}
		return []byte{o.op, n}, nil
	case formRNN:
		r, err := reg(parts[1])
		if err != nil {
			return nil, err
		}
		operand := step
		if o.mnemonic == "LDV" {
			operand = val
		}
		n, err := operand(parts[2])
		if err != nil {
			return nil, err
		}
		return []byte{o.op | r, n}, nil
	}
	return []byte{o.op}, nil
}

// --- decimal (BCD) helpers -------------------------------------------------
//
// Program steps are numbered 00-99 and branch targets are written as the
// step number, so GTO.15 is encoded 12 15: addresses are binary-coded
// decimal. Arithmetic is decimal too (9+9 unpacks to the digits 1 and 8).

// bcdToStep converts a BCD step number such as 0x40 to 40.
func bcdToStep(b byte) int {
	return (int(b>>4)*10 + int(b&0x0F)) % Steps
}

// stepToBCD converts a step number such as 40 to 0x40.
func stepToBCD(n int) byte {
	n = ((n % Steps) + Steps) % Steps
	return byte(n/10<<4 | n%10)
}

// bcdAdd adds two BCD bytes the way the 8048's ADD + DA A sequence does,
// keeping only the lowest two digits.
func bcdAdd(a, b byte) byte {
	sum := int(a) + int(b)
	halfCarry := int(a&0x0F)+int(b&0x0F) > 0x0F
	carry := sum > 0xFF
	r := sum & 0xFF
	if r&0x0F > 9 || halfCarry {
		r += 0x06
		if r > 0xFF {
			carry = true
		}
		r &= 0xFF
	}
	if r>>4 > 9 || carry {
		r = (r + 0x60) & 0xFF
	}
	return byte(r)
}

// bcdSub returns minuend - subtrahend in decimal, wrapping 00-1 to 99.
func bcdSub(minuend, subtrahend byte) byte {
	lo := int(minuend&0x0F) - int(subtrahend&0x0F)
	borrow := 0
	if lo < 0 {
		lo += 10
		borrow = 1
	}
	hi := int(minuend>>4) - int(subtrahend>>4) - borrow
	if hi < 0 {
		hi += 10
	}
	return byte((hi&0x0F)<<4 | lo&0x0F)
}
