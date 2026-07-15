package audio

// Resampler performs stateful, streaming sample-rate conversion of a mono
// float32 signal using linear interpolation. It is the streaming counterpart
// to ResampleLinear: where ResampleLinear treats its input as a complete,
// self-contained clip, Resampler remembers the fractional read position and
// the trailing input sample across Process calls. Feeding a signal to Process
// in arbitrarily sized blocks therefore yields the same output samples as
// resampling the whole signal in one call — no per-block discontinuity and no
// cumulative drift on non-integer ratios (e.g. 44100 -> 16000).
//
// Like ResampleLinear it does no anti-alias filtering, which is adequate for
// whisper input (trained on 16 kHz). A Resampler is not safe for concurrent
// use; drive it from a single goroutine.
type Resampler struct {
	inRate  int
	outRate int
	ratio   float64 // inRate / outRate: input samples advanced per output sample

	// pos is the read position of the next output sample, in input-sample
	// units relative to the start of the next Process block. It is carried
	// across calls and is in the range [-1, 0) at the start of every block
	// after the first, where index -1 refers to last.
	pos float64

	// last is the final input sample of the previous block, acting as input
	// index -1 for interpolation at the head of the next block.
	last float32

	// primed reports whether last holds a real sample yet (false until the
	// first non-empty block has been processed).
	primed bool
}

// NewResampler returns a Resampler that converts from inRate to outRate. Both
// rates must be positive. When inRate == outRate, Process returns its input
// unchanged.
func NewResampler(inRate, outRate int) *Resampler {
	return &Resampler{
		inRate:  inRate,
		outRate: outRate,
		ratio:   float64(inRate) / float64(outRate),
	}
}

// Reset clears the carried phase and trailing-sample state so the Resampler
// can be reused for a fresh, unrelated stream (e.g. after a session reset)
// without allocating a new one.
func (r *Resampler) Reset() {
	r.pos = 0
	r.last = 0
	r.primed = false
}

// Process resamples one block of mono input and returns the output samples
// produced from it. State is carried so the next call continues seamlessly.
// The returned slice is freshly allocated; in is not modified. A nil or empty
// in returns nil. When inRate == outRate the input is returned as-is.
func (r *Resampler) Process(in []float32) []float32 {
	if len(in) == 0 {
		return nil
	}
	if r.inRate == r.outRate {
		return in
	}

	n := len(in)

	// Worst-case output count is len(in)/ratio + 1; preallocate to avoid
	// repeated growth.
	out := make([]float32, 0, int(float64(n)/r.ratio)+1)

	for {
		i := floor(r.pos)
		frac := float32(r.pos - float64(i))

		var a, b float32
		switch {
		case i < 0:
			// Reading between the previous block's last sample (index -1)
			// and this block's first sample.
			if !r.primed {
				// First ever block: no prior sample to interpolate from.
				// pos starts at 0, so this branch should not run, but guard
				// against a negative start defensively.
				a, b = in[0], in[0]
			} else {
				a, b = r.last, in[0]
			}

		case i+1 < n:
			a, b = in[i], in[i+1]

		default:
			// Need in[n] (the next block's first sample); stop and carry.
			goto done
		}

		out = append(out, a*(1-frac)+b*frac)
		r.pos += r.ratio
	}

done:
	// Rebase pos to be relative to the start of the next block and remember
	// this block's final sample as the new index -1.
	r.pos -= float64(n)
	r.last = in[n-1]
	r.primed = true

	return out
}

// floor returns the floor of f as an int. f is small and bounded by block
// length, so a direct conversion after adjusting for negatives is exact.
func floor(f float64) int {
	i := int(f)
	if float64(i) > f {
		i--
	}
	return i
}
