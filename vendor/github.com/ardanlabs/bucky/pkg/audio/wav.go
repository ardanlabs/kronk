package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// WAV format codes.
const (
	wavFormatPCM        = 1
	wavFormatIEEEFloat  = 3
	wavFormatExtensible = 0xFFFE
)

// DecodeWAV reads a RIFF/WAVE stream and returns interleaved float32 samples
// in the [-1, 1] range, plus the file's sample rate and channel count.
//
// Supported encodings: 8-bit unsigned PCM, 16/24/32-bit signed PCM, and
// 32-bit IEEE float. Other encodings (A-law, mu-law, ADPCM, etc.) return
// an error.
func DecodeWAV(r io.Reader) ([]float32, int, int, error) {
	return DecodeWAVInto(nil, r)
}

// DecodeWAVInto is the buffer-reusing form of DecodeWAV. When dst has
// enough capacity for all decoded samples, the returned slice shares dst's
// backing array and no []float32 allocation happens. See audio.DecodeInto
// for usage. dst may be nil.
func DecodeWAVInto(dst []float32, r io.Reader) ([]float32, int, int, error) {
	var hdr struct {
		Riff      [4]byte
		ChunkSize uint32
		Wave      [4]byte
	}
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, 0, 0, err
	}
	if string(hdr.Riff[:]) != "RIFF" || string(hdr.Wave[:]) != "WAVE" {
		return nil, 0, 0, errors.New("audio: not a RIFF/WAVE file")
	}

	info := wavInfo{dst: dst}
	for {
		var sub struct {
			ID   [4]byte
			Size uint32
		}
		if err := binary.Read(r, binary.LittleEndian, &sub); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, 0, 0, err
		}
		done, err := readWAVChunk(r, sub.ID, sub.Size, &info)
		if err != nil {
			return nil, 0, 0, err
		}
		if done {
			break
		}
	}

	if info.samples == nil {
		return nil, 0, 0, errors.New("audio: WAV had no data chunk")
	}
	return info.samples, int(info.sampleRate), int(info.channels), nil
}

// wavInfo accumulates parsed fmt and data chunk results across the chunk
// scan loop in DecodeWAV. dst is the caller-supplied output buffer
// (possibly nil); samples is the final result, which shares dst's backing
// array when dst has enough capacity.
type wavInfo struct {
	fmtFound      bool
	audioFormat   uint16
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
	dst           []float32
	samples       []float32
}

// readWAVChunk dispatches a single sub-chunk and reports whether DecodeWAV
// should stop scanning (true once data has been read).
func readWAVChunk(r io.Reader, id [4]byte, size uint32, info *wavInfo) (bool, error) {
	switch string(id[:]) {
	case "fmt ":
		return false, readWAVFmt(r, size, info)
	case "data":
		return readWAVData(r, size, info)
	default:
		return false, skipWAVChunk(r, size)
	}
}

func readWAVFmt(r io.Reader, size uint32, info *wavInfo) error {
	var fmtChunk struct {
		AudioFormat   uint16
		NumChannels   uint16
		SampleRate    uint32
		ByteRate      uint32
		BlockAlign    uint16
		BitsPerSample uint16
	}
	if err := binary.Read(r, binary.LittleEndian, &fmtChunk); err != nil {
		return err
	}
	info.audioFormat = fmtChunk.AudioFormat
	info.channels = fmtChunk.NumChannels
	info.sampleRate = fmtChunk.SampleRate
	info.bitsPerSample = fmtChunk.BitsPerSample

	// WAVEFORMATEX / EXTENSIBLE adds extra bytes after the basic 16.
	if extra := int64(size) - 16; extra > 0 {
		skip := make([]byte, extra)
		if _, err := io.ReadFull(r, skip); err != nil {
			return err
		}
		// For WAVE_FORMAT_EXTENSIBLE, the real format lives in the
		// SubFormat GUID's first 16-bit field.
		if info.audioFormat == wavFormatExtensible && len(skip) >= 24 {
			info.audioFormat = binary.LittleEndian.Uint16(skip[8:10])
		}
	}
	info.fmtFound = true
	return nil
}

func readWAVData(r io.Reader, size uint32, info *wavInfo) (bool, error) {
	if !info.fmtFound {
		return false, errors.New("audio: data chunk before fmt chunk")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return false, err
	}
	s, err := decodeWAVData(info.dst, data, info.audioFormat, info.bitsPerSample)
	if err != nil {
		return false, err
	}
	info.samples = s
	// WAV chunks are 2-byte aligned; consume an odd-length pad byte.
	if size%2 == 1 {
		var pad [1]byte
		_, _ = io.ReadFull(r, pad[:])
	}
	return true, nil
}

func skipWAVChunk(r io.Reader, size uint32) error {
	skip := make([]byte, size)
	if _, err := io.ReadFull(r, skip); err != nil {
		// Unknown trailing chunk truncated at EOF: treat as end of file
		// rather than a hard error.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if size%2 == 1 {
		var pad [1]byte
		_, _ = io.ReadFull(r, pad[:])
	}
	return nil
}

// decodeWAVData converts the raw bytes of a WAV "data" chunk into float32
// samples in [-1, 1]. The result is written into dst's backing array when
// dst has enough capacity; otherwise a fresh slice is allocated. Pass
// dst=nil to always allocate.
func decodeWAVData(dst []float32, data []byte, audioFormat, bitsPerSample uint16) ([]float32, error) {
	switch audioFormat {
	case wavFormatPCM:
		switch bitsPerSample {
		case 8:
			out := wavOutputBuf(dst, len(data))
			for i, b := range data {
				// 8-bit PCM is unsigned with center at 128.
				out[i] = (float32(int(b) - 128)) / 128.0
			}
			return out, nil
		case 16:
			n := len(data) / 2
			out := wavOutputBuf(dst, n)
			for i := range n {
				v := int16(binary.LittleEndian.Uint16(data[i*2:]))
				out[i] = float32(v) / 32768.0
			}
			return out, nil
		case 24:
			n := len(data) / 3
			out := wavOutputBuf(dst, n)
			for i := range n {
				b0 := uint32(data[i*3])
				b1 := uint32(data[i*3+1])
				b2 := uint32(data[i*3+2])
				v := int32(b0 | b1<<8 | b2<<16)
				if v&0x800000 != 0 {
					v |= ^0xFFFFFF // sign-extend
				}
				out[i] = float32(v) / 8388608.0
			}
			return out, nil
		case 32:
			n := len(data) / 4
			out := wavOutputBuf(dst, n)
			for i := range n {
				v := int32(binary.LittleEndian.Uint32(data[i*4:]))
				out[i] = float32(v) / 2147483648.0
			}
			return out, nil
		}
	case wavFormatIEEEFloat:
		if bitsPerSample == 32 {
			n := len(data) / 4
			out := wavOutputBuf(dst, n)
			for i := range n {
				bits := binary.LittleEndian.Uint32(data[i*4:])
				out[i] = math.Float32frombits(bits)
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("audio: unsupported WAV encoding format=%d bps=%d", audioFormat, bitsPerSample)
}

// wavOutputBuf returns a length-n []float32 backed by dst when dst has
// enough capacity, or a freshly allocated slice otherwise. Lets the
// per-format decode loops avoid the per-call allocation when the caller
// supplied a reusable buffer through DecodeInto / DecodeWAVInto.
func wavOutputBuf(dst []float32, n int) []float32 {
	if cap(dst) >= n {
		return dst[:n]
	}
	return make([]float32, n)
}
