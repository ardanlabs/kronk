package gguf

import "testing"

func FuzzParsers(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{'G', 'G', 'U', 'F'})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseMetadata(data)
		_, _, _ = ParseHeaderAndTensors(data, int64(len(data)))
	})
}
