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

// Opcodes. An instruction with a register operand has the register in its
// low digit: OUT.5 is opOUT|5 = C5.
const (
	opNOP byte = 0x00
	opCLR byte = 0x01
	opDEC byte = 0x02
	opINC byte = 0x03
	opINA byte = 0x04
	opSIG byte = 0x05
	opRET byte = 0x07
	opRND byte = 0x08
	opMOV byte = 0x09
	opOTA byte = 0x0B
	opBDB byte = 0x10
	opBDC byte = 0x11
	opGTO byte = 0x12
	opBRZ byte = 0x13
	opGTS byte = 0x14
	opBNE byte = 0x20
	opBEQ byte = 0x30
	opBGT byte = 0x40
	opBLS byte = 0x50
	opLDV byte = 0x60
	opINP byte = 0x70
	opPAK byte = 0x80
	opLDA byte = 0x90
	opSTO byte = 0xA0
	opUNP byte = 0xB0
	opOUT byte = 0xC0
	opSUB byte = 0xD0
	opADD byte = 0xE0
	opHLT byte = 0xFF
)

type opInfo struct {
	mnemonic string
	op       byte
	form     form
}

// size is the number of bytes (program steps) the instruction occupies.
func (o opInfo) size() int {
	if o.form == formNN || o.form == formRNN {
		return 2
	}
	return 1
}

var opTable = []opInfo{
	{"NOP", opNOP, formNone},
	{"CLR", opCLR, formNone},
	{"DEC", opDEC, formNone},
	{"INC", opINC, formNone},
	{"INA", opINA, formNone},
	{"SIG", opSIG, formNone},
	{"RET", opRET, formNone},
	{"RND", opRND, formNone},
	{"MOV", opMOV, formNone},
	{"OTA", opOTA, formNone},
	{"HLT", opHLT, formNone},
	{"BDB", opBDB, formNN},
	{"BDC", opBDC, formNN},
	{"GTO", opGTO, formNN},
	{"BRZ", opBRZ, formNN},
	{"GTS", opGTS, formNN},
	{"BNE", opBNE, formRNN},
	{"BEQ", opBEQ, formRNN},
	{"BGT", opBGT, formRNN},
	{"BLS", opBLS, formRNN},
	{"LDV", opLDV, formRNN},
	{"INP", opINP, formR},
	{"PAK", opPAK, formR},
	{"LDA", opLDA, formR},
	{"STO", opSTO, formR},
	{"UNP", opUNP, formR},
	{"OUT", opOUT, formR},
	{"SUB", opSUB, formR},
	{"ADD", opADD, formR},
}

var opByMnemonic = func() map[string]opInfo {
	m := map[string]opInfo{}
	for _, o := range opTable {
		m[o.mnemonic] = o
	}
	m["BVNE"] = m["BNE"] // spelling used in the manual's instruction chart
	return m
}()

// opByCode maps every byte to its instruction. Undefined opcodes have an
// empty mnemonic.
var opByCode = func() (m [256]opInfo) {
	for _, o := range opTable {
		switch o.form {
		case formNone, formNN:
			m[o.op] = o
		case formR, formRNN:
			for r := range byte(16) {
				m[o.op|r] = o
			}
		}
	}
	return m
}()

// decode returns the instruction descriptor for an opcode byte.
func decode(op byte) (opInfo, bool) {
	o := opByCode[op]
	return o, o.mnemonic != ""
}

// Disassemble renders the instruction at step in assembler notation and
// returns the number of steps it occupies from step onward. A two-byte
// instruction at step 99 takes its operand from step 00, which a listing
// shows on a line of its own, so it counts as one step. Unknown opcodes
// render as "??", followed by the letter when the byte is a letter's
// character code (message data).
func Disassemble(mem *[Steps]byte, step int) (string, int) {
	step = (step%Steps + Steps) % Steps
	op := mem[step]
	o, ok := decode(op)
	if !ok {
		if r, ok := codeLetters[op]; ok {
			return fmt.Sprintf("?? '%c'", r), 1
		}
		return "??", 1
	}
	nn := mem[(step+1)%Steps]
	r := op & 0x0F
	n := min(o.size(), Steps-step)
	// A branch target that isn't a step number (the assembler rejects
	// GTO.A5) still jumps somewhere: show where, as "GTO.A5 (=05)".
	target := ""
	if o.op != opLDV && (nn>>4 > 9 || nn&0x0F > 9) {
		target = fmt.Sprintf(" (=%02d)", bcdToStep(nn))
	}
	switch o.form {
	case formR:
		return fmt.Sprintf("%s.%X", o.mnemonic, r), n
	case formNN:
		return fmt.Sprintf("%s.%02X%s", o.mnemonic, nn, target), n
	case formRNN:
		return fmt.Sprintf("%s.%X.%02X%s", o.mnemonic, r, nn, target), n
	}
	return o.mnemonic, n
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
			return 0, fmt.Errorf("value must be one or two hex digits, got %q", s)
		}
		v, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			return 0, fmt.Errorf("value must be one or two hex digits, got %q", s)
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
		if o.op == opLDV {
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
