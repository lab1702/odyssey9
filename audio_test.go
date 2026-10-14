package main

import (
	"testing"
	"time"

	"odyssey9/machine"
)

// playing returns a stream playing snd for a long time.
func playing(start float64, snd machine.Sound) *tone {
	s := &tone{t: start}
	s.set(snd, time.Hour)
	s.refresh(time.Now())
	return s
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
	s.set(machine.SoundBuzz, 50*time.Millisecond)
	s.refresh(time.Now())
	for range sampleRate / 20 { // 50 ms
		s.sample()
	}
	if s.vol < 0.1 {
		t.Fatalf("buzz silent before its end (volume %g)", s.vol)
	}
	for range sampleRate / 20 {
		s.sample()
	}
	if s.vol > 0.001 {
		t.Errorf("buzz still sounding 50 ms after its end (volume %g)", s.vol)
	}
}
