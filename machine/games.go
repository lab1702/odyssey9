package machine

import (
	"sort"
	"time"
)

// game is one of the four programs built into the cartridge, started by
// pressing 1-4 from the title screen or the Command mode.
type game interface {
	key(m *Machine, k Key)
	tick(m *Machine, dt time.Duration)
}

func (m *Machine) startGame(n int) {
	m.clearScreen()
	m.mode = ModeGame
	m.showHalt = false
	switch n {
	case 1:
		m.game = &creepyCrawler{}
	case 2:
		g := &flashCard{}
		g.newProblem(m)
		m.game = g
	case 3:
		g := &telepathy{}
		g.newNumber(m)
		m.game = g
	case 4:
		g := &betweenSheets{}
		g.newRound(m)
		m.game = g
	}
}

func (m *Machine) put(pos int, codes ...byte) {
	for i, c := range codes {
		if p := pos + i; p >= 0 && p < Positions {
			m.Screen[p] = c
		}
	}
}

func digits(n int) (byte, byte) { return byte(n / 10 % 10), byte(n % 10) }

// --- 1: The Creepy Crawler ---------------------------------------------------
// "A series of blocks appears on the screen. The blocks flash in random order
// and are accompanied by a buzzing sound."

type creepyCrawler struct{ next time.Duration }

func (g *creepyCrawler) key(m *Machine, k Key) {}

func (g *creepyCrawler) tick(m *Machine, dt time.Duration) {
	g.next -= dt
	if g.next > 0 {
		return
	}
	g.next = time.Duration(120+m.rng.Intn(180)) * time.Millisecond
	p := m.rng.Intn(Positions)
	if m.Screen[p] == blank {
		m.Screen[p] = 0x2F
	} else {
		m.Screen[p] = blank
	}
	m.play(SoundBuzz, g.next*2/3)
}

// --- 2: Flash Card --------------------------------------------------------
// An unsolved addition problem flashes on the screen. Enter the two-digit
// answer; a wrong answer shows an angry NO, a right one appears in place.
// Press any key for the next problem.

type flashCard struct {
	a, b    int
	answer  []int
	solved  bool
	blinkOn bool
	blink   time.Duration
}

func (g *flashCard) newProblem(m *Machine) {
	g.a, g.b = m.rng.Intn(10), m.rng.Intn(10)
	g.answer = nil
	g.solved = false
	g.blinkOn = true
	g.blink = 600 * time.Millisecond
	m.clearScreen()
	g.drawProblem(m, true)
}

func (g *flashCard) drawProblem(m *Machine, on bool) {
	if on {
		m.put(0, byte(g.a), 0x10, byte(g.b), 0x2B)
	} else {
		m.put(0, blank, blank, blank, blank)
	}
}

func (g *flashCard) key(m *Machine, k Key) {
	if g.solved {
		g.newProblem(m)
		return
	}
	d := k.Digit()
	if d < 0 {
		m.errorBeep()
		return
	}
	if len(g.answer) == 0 {
		m.put(4, blank, blank, blank, blank, blank) // clear old guess and NO
	}
	g.answer = append(g.answer, d)
	m.put(3+len(g.answer), byte(d))
	m.play(SoundBeep, 60*time.Millisecond)
	if len(g.answer) < 2 {
		return
	}
	if g.answer[0]*10+g.answer[1] == g.a+g.b {
		g.solved = true
		g.drawProblem(m, true)
		m.play(SoundChirp, 300*time.Millisecond)
	} else {
		m.put(7, 0x2D, 0x17) // NO
		m.play(SoundError, 400*time.Millisecond)
	}
	g.answer = nil
}

func (g *flashCard) tick(m *Machine, dt time.Duration) {
	if g.solved {
		return
	}
	g.blink -= dt
	if g.blink <= 0 {
		g.blinkOn = !g.blinkOn
		if g.blinkOn {
			g.blink = 600 * time.Millisecond
		} else {
			g.blink = 250 * time.Millisecond
		}
		g.drawProblem(m, g.blinkOn)
	}
}

// --- 3: Computer Telepathy -------------------------------------------------
// The computer secretly chooses a number between 00 and 99. Your guess is
// followed by H if it is higher than the secret, L if lower, X if correct.

type telepathy struct {
	secret int
	guess  []int
	won    bool
}

func (g *telepathy) newNumber(m *Machine) {
	g.secret = m.rng.Intn(100)
	g.guess = nil
	g.won = false
	m.clearScreen()
	m.put(0, 0x0D) // ?
}

func (g *telepathy) key(m *Machine, k Key) {
	if g.won {
		g.newNumber(m)
		return
	}
	d := k.Digit()
	if d < 0 {
		m.errorBeep()
		return
	}
	if len(g.guess) == 0 {
		m.clearScreen()
	}
	g.guess = append(g.guess, d)
	m.put(len(g.guess)-1, byte(d))
	m.play(SoundBeep, 60*time.Millisecond)
	if len(g.guess) < 2 {
		return
	}
	n := g.guess[0]*10 + g.guess[1]
	g.guess = nil
	switch {
	case n > g.secret:
		m.put(3, 0x1D) // H
	case n < g.secret:
		m.put(3, 0x0E) // L
	default:
		m.put(3, 0x22) // X
		g.won = true
		m.play(SoundChirp, 500*time.Millisecond)
	}
}

func (g *telepathy) tick(m *Machine, dt time.Duration) {}

// --- 4: Between the Sheets -------------------------------------------------
// Two numbers (the sheets) and your score are shown, e.g. 03 07 00. Press YES
// if you think the computer's secret number lies between the sheets, NO if
// not. A right answer scores a point and the secret appears in its place
// among the sheets (03 05 07 01, or 03 07 50 01 after a right NO); a wrong
// one earns a couple of beeps.

type betweenSheets struct {
	lo, hi, secret, score int
	waiting               bool
	pause                 time.Duration
}

func (g *betweenSheets) newRound(m *Machine) {
	g.lo = m.rng.Intn(80)
	g.hi = g.lo + 2 + m.rng.Intn(18)
	// Half the time the secret lies between the sheets, so YES and NO are
	// equally good guesses. It never equals a sheet, which would leave it
	// unclear which side it is on.
	if inside := g.hi - g.lo - 1; m.rng.Intn(2) == 0 {
		g.secret = g.lo + 1 + m.rng.Intn(inside)
	} else if g.secret = m.rng.Intn(100 - inside - 2); g.secret >= g.lo {
		g.secret += inside + 2
	}
	g.waiting = true
	m.clearScreen()
	l1, l2 := digits(g.lo)
	h1, h2 := digits(g.hi)
	s1, s2 := digits(g.score)
	m.put(0, l1, l2, blank, h1, h2, blank, s1, s2)
}

func (g *betweenSheets) key(m *Machine, k Key) {
	if !g.waiting {
		return
	}
	if k != KeyYes && k != KeyNo {
		m.errorBeep()
		return
	}
	between := g.secret > g.lo && g.secret < g.hi
	g.waiting = false
	g.pause = 2 * time.Second
	if between == (k == KeyYes) {
		g.score = (g.score + 1) % 100
		n := []int{g.lo, g.secret, g.hi}
		sort.Ints(n)
		a1, a2 := digits(n[0])
		b1, b2 := digits(n[1])
		c1, c2 := digits(n[2])
		s1, s2 := digits(g.score)
		m.put(0, a1, a2, blank, b1, b2, blank, c1, c2, blank, s1, s2)
		m.play(SoundChirp, 300*time.Millisecond)
	} else {
		g.pause = 1500 * time.Millisecond
		m.play(SoundBeeps, 600*time.Millisecond)
	}
}

func (g *betweenSheets) tick(m *Machine, dt time.Duration) {
	if g.waiting {
		return
	}
	g.pause -= dt
	if g.pause <= 0 {
		g.newRound(m)
	}
}
