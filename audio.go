package main

import (
	"fmt"
	"math"
	"os"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"odyssey9/machine"
)

const sampleRate = 48000

// tone is an endless 16-bit stereo stream whose waveform follows the
// machine's current sound.
type tone struct {
	sound atomic.Int32
	cur   machine.Sound // the waveform being generated
	t     float64       // seconds since start
	start float64       // value of t when cur began
	phase float64       // square wave position in its cycle, 0 to 1
	vol   float64       // smoothed volume to avoid clicks
	lfsr  uint32
	noise float64
	nextN float64

	// The player closes itself when garbage collected, so the stream keeps
	// a reference to it for as long as the game holds the stream.
	player *audio.Player
}

func (s *tone) Read(p []byte) (int, error) {
	n := len(p) / 4 * 4
	for i := 0; i < n; i += 4 {
		v := s.sample()
		x := int16(v * 32767)
		p[i], p[i+1], p[i+2], p[i+3] = byte(x), byte(x>>8), byte(x), byte(x>>8)
	}
	return n, nil
}

// square advances the square wave by one sample at frequency f and returns
// its value. Keeping the phase, rather than deriving it from t, means a
// change of frequency never makes the wave jump.
func (s *tone) square(f float64) float64 {
	s.phase += f / sampleRate
	s.phase -= math.Floor(s.phase)
	if s.phase < 0.5 {
		return 1
	}
	return -1
}

func (s *tone) sample() float64 {
	const dt = 1.0 / sampleRate
	s.t += dt
	// When the sound stops or changes, keep the old waveform going while its
	// volume fades out, so the output never jumps (which would click).
	next := machine.Sound(s.sound.Load())
	fading := false
	if next != s.cur {
		if s.vol > 0.001 {
			fading = true
		} else {
			s.cur, s.start = next, s.t
		}
	}
	v, target := s.wave()
	if fading {
		target = 0
	}
	s.vol += (target - s.vol) * 0.004
	return v * s.vol
}

// wave returns the current waveform's next value and its volume.
func (s *tone) wave() (v, target float64) {
	switch s.cur {
	case machine.SoundBeep:
		return s.square(988), 0.22
	case machine.SoundBuzz:
		// The Odyssey2 buzz: a low square wave roughened by a noise shift
		// register.
		if s.t >= s.nextN {
			s.nextN = s.t + 1.0/1800
			if s.lfsr == 0 {
				s.lfsr = 0xACE1
			}
			bit := (s.lfsr ^ s.lfsr>>2 ^ s.lfsr>>3 ^ s.lfsr>>5) & 1
			s.lfsr = s.lfsr>>1 | bit<<15
			s.noise = float64(bit)*2 - 1
		}
		return 0.6*s.square(98) + 0.4*s.noise, 0.25
	case machine.SoundError:
		return s.square(165), 0.22
	case machine.SoundChirp:
		return s.square(1320 + 220*math.Sin(s.t*40)), 0.18
	case machine.SoundBeeps:
		// Six beeps a second, the first starting with the sound.
		if math.Mod((s.t-s.start)*6, 1) < 0.55 {
			target = 0.22
		}
		return s.square(1175), target
	}
	return 0, 0
}

// newAudio starts the sound output, or returns nil if audio is disabled. A
// missing or broken sound device isn't detected here: the audio library
// reports it later, as an error that stops the game (see main).
func newAudio(enabled bool) *tone {
	if !enabled {
		return nil
	}
	ctx := audio.NewContext(sampleRate)
	t := &tone{}
	p, err := ctx.NewPlayer(t)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No sound:", err)
		return nil
	}
	p.SetBufferSize(40 * time.Millisecond)
	p.Play()
	t.player = p
	return t
}

func (s *tone) set(snd machine.Sound) {
	if s != nil {
		s.sound.Store(int32(snd))
	}
}
