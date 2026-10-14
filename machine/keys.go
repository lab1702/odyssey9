package machine

// Key identifies one physical key on the Odyssey2 keyboard.
type Key int

const (
	KeyNone Key = iota
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeyPlus
	KeyMinus
	KeyTimes
	KeyDivide
	KeyEquals
	KeyYes
	KeyNo
	KeyClear
	KeyEnter
	KeyReset
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	KeyPeriod
	KeyQuestion
	KeySpace
	keyCount
)

// keyCodes is the "Key Codes" table from the manual's front gatefold. The
// table misprints Z as "31 / 32" (31 is the ball, and 32 decimal is A): Z's
// character is 21, as in the glyph table in font.go.
var keyCodes = [keyCount]byte{
	Key0: 0x00, Key1: 0x01, Key2: 0x02, Key3: 0x03, Key4: 0x04,
	Key5: 0x05, Key6: 0x06, Key7: 0x07, Key8: 0x08, Key9: 0x09,
	KeyPlus: 0x10, KeyMinus: 0x28, KeyTimes: 0x29, KeyDivide: 0x2A,
	KeyEquals: 0x2B, KeyYes: 0x2C, KeyNo: 0x2D, KeyClear: 0x2E,
	KeyEnter: 0x2F, KeyReset: 0xFF,
	KeyA: 0x20, KeyB: 0x25, KeyC: 0x23, KeyD: 0x1A, KeyE: 0x12, KeyF: 0x1B,
	KeyG: 0x1C, KeyH: 0x1D, KeyI: 0x16, KeyJ: 0x1E, KeyK: 0x1F, KeyL: 0x0E,
	KeyM: 0x26, KeyN: 0x2D, KeyO: 0x17, KeyP: 0x0F, KeyQ: 0x18, KeyR: 0x13,
	KeyS: 0x19, KeyT: 0x14, KeyU: 0x15, KeyV: 0x24, KeyW: 0x11, KeyX: 0x22,
	KeyY: 0x2C, KeyZ: 0x21,
	KeyPeriod: 0x27, KeyQuestion: 0x0D, KeySpace: 0x0C,
}

// Code returns the value a key delivers to a program (INP/INA).
func (k Key) Code() byte {
	if k <= KeyNone || k >= keyCount {
		return 0x0C
	}
	return keyCodes[k]
}

// Digit returns the numeric value of a 0-9 key, or -1.
func (k Key) Digit() int {
	if k >= Key0 && k <= Key9 {
		return int(k - Key0)
	}
	return -1
}

// HexDigit returns the value of a 0-9 or A-F key, or -1.
func (k Key) HexDigit() int {
	if d := k.Digit(); d >= 0 {
		return d
	}
	if k >= KeyA && k <= KeyF {
		return int(k-KeyA) + 10
	}
	return -1
}

// Rune returns the character a key types in the assembler, or 0.
func (k Key) Rune() rune {
	switch {
	case k >= Key0 && k <= Key9:
		return rune('0' + k - Key0)
	case k >= KeyA && k <= KeyZ:
		return rune('A' + k - KeyA)
	case k == KeyPeriod:
		return '.'
	}
	return 0
}

// KeyForLetter returns the key for an ASCII letter or digit, or KeyNone.
func KeyForLetter(r rune) Key {
	switch {
	case r >= 'a' && r <= 'z':
		return KeyA + Key(r-'a')
	case r >= 'A' && r <= 'Z':
		return KeyA + Key(r-'A')
	case r >= '0' && r <= '9':
		return Key0 + Key(r-'0')
	}
	return KeyNone
}
