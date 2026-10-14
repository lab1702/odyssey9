package main

import (
	"testing"
	"time"

	"odyssey9/machine"
)

// playing returns a stream playing snd for a long time.
func playing(start float64, snd machine.Sound) *tone {
	s := &tone{t: start}
	s.set(snd, time.Hour, true)
	s.pickUp()
	return s
}

// run generates d of the stream, picking up new sounds as Read does.
func (s *tone) run(d time.Duration) {
	s.pickUp()
	for range int(d.Seconds() * sampleRate) {
		s.sample()
	}
}

// The chirp's pitch must keep warbling around 1320 Hz however long the
// stream has been running.
func TestChirpFrequencyStaysInRange(t *testing.T) {
	for _, start := range []float64{0, 600} {
		s := playing(start, machine.SoundChirp)
		const window = sampleRate / 50 // 20 ms
		prev := 0.0
		for range 25 {
			rises := 0
			for range window {
				v := s.sample()
				if prev <= 0 && v > 0 {
					rises++
				}
				prev = v
			}
			// Allow a cycle either way for where the window cuts the wave.
			if f := float64(rises) * 50; f < 1100-50 || f > 1540+50 {
				t.Fatalf("after %gs of stream: chirp at %g Hz", start, f)
			}
		}
	}
}

// The first of the repeating beeps must sound as soon as the sound starts,
// whatever the stream time.
func TestBeepsStartOn(t *testing.T) {
	for _, start := range []float64{0, 0.1, 600.1} {
		s := playing(start, machine.SoundBeeps)
		for range sampleRate / 100 { // 10 ms
			s.sample()
		}
		if s.vol < 0.1 {
			t.Errorf("after %gs of stream: beeps silent 10 ms after starting (volume %g)", start, s.vol)
		}
	}
}

// A sound ends on time even when the game loop, which would stop it, is
// held up.
func TestSoundEndsWithoutGameLoop(t *testing.T) {
	s := &tone{}
	s.set(machine.SoundBuzz, 50*time.Millisecond, false)
	s.run(50 * time.Millisecond)
	if s.vol < 0.1 {
		t.Fatalf("buzz silent before its end (volume %g)", s.vol)
	}
	s.run(50 * time.Millisecond)
	if s.vol > 0.001 {
		t.Errorf("buzz still sounding 50 ms after its end (volume %g)", s.vol)
	}
}

// A sound plays for its whole length however late after it starts the
// stream picks it up, and the machine's own end of the sound, which comes
// that much sooner, doesn't cut it short.
func TestSoundLastsWhenPickedUpLate(t *testing.T) {
	s := &tone{}
	s.set(machine.SoundBeep, 60*time.Millisecond, true)
	s.pickUp()
	// The game loop counts the beep down and ends it, and goes on reporting
	// no sound, before the stream has played it all.
	for left := 60 * time.Millisecond; left > 0; left -= 16 * time.Millisecond {
		s.set(machine.SoundBeep, left, false)
	}
	s.set(machine.SoundNone, 0, false)
	s.set(machine.SoundNone, 0, true)
	s.run(55 * time.Millisecond)
	if s.vol < 0.1 {
		t.Fatalf("beep silent 55 ms into a 60 ms beep (volume %g)", s.vol)
	}
	s.run(50 * time.Millisecond)
	if s.vol > 0.001 {
		t.Errorf("beep still sounding 45 ms after its end (volume %g)", s.vol)
	}
}

// RESET or POWER stops a sound at once.
func TestSoundCut(t *testing.T) {
	s := playing(0, machine.SoundBuzz)
	s.run(20 * time.Millisecond)
	s.set(machine.SoundNone, 0, true)
	s.run(40 * time.Millisecond) // fading out takes about 30 ms
	if s.vol > 0.001 {
		t.Errorf("buzz still sounding 40 ms after being cut (volume %g)", s.vol)
	}
}

// Playing the same sound again restarts it, but the game loop reporting a
// sound counting down doesn't.
func TestSoundPlayedAgain(t *testing.T) {
	s := &tone{}
	s.set(machine.SoundBeep, 60*time.Millisecond, true)
	s.run(40 * time.Millisecond)
	s.set(machine.SoundBeep, 20*time.Millisecond, false)
	s.run(10 * time.Millisecond)
	if s.left != sampleRate/100 {
		t.Fatalf("beep restarted by its countdown: %d samples left, want %d", s.left, sampleRate/100)
	}
	s.set(machine.SoundBeep, 60*time.Millisecond, false)
	s.pickUp()
	if s.left != 60*sampleRate/1000 {
		t.Errorf("beep played again has %d samples left, want %d", s.left, 60*sampleRate/1000)
	}
}
