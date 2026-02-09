package model

import (
	"testing"
)

func TestExtractCompleteUTF8(t *testing.T) {
	tests := []struct {
		name          string
		input         []byte
		wantComplete  string
		wantRemainLen int
	}{
		{
			name:          "pure ASCII",
			input:         []byte("hello"),
			wantComplete:  "hello",
			wantRemainLen: 0,
		},
		{
			name:          "complete multi-byte (checkmark)",
			input:         []byte("✅"),
			wantComplete:  "✅",
			wantRemainLen: 0,
		},
		{
			name:          "partial 3-byte char (first 2 bytes of ✅ = E2 9C 85)",
			input:         []byte{0xE2, 0x9C},
			wantComplete:  "",
			wantRemainLen: 2,
		},
		{
			name:          "partial 3-byte char (first byte only)",
			input:         []byte{0xE2},
			wantComplete:  "",
			wantRemainLen: 1,
		},
		{
			name:          "ASCII then partial multi-byte",
			input:         []byte{'h', 'i', 0xE2, 0x9C},
			wantComplete:  "hi",
			wantRemainLen: 2,
		},
		{
			name:          "partial 4-byte emoji (first 2 bytes of 🔥 = F0 9F 94 A5)",
			input:         []byte{0xF0, 0x9F},
			wantComplete:  "",
			wantRemainLen: 2,
		},
		{
			name:          "partial 4-byte emoji (first 3 bytes)",
			input:         []byte{0xF0, 0x9F, 0x94},
			wantComplete:  "",
			wantRemainLen: 3,
		},
		{
			name:          "complete 4-byte emoji",
			input:         []byte("🔥"),
			wantComplete:  "🔥",
			wantRemainLen: 0,
		},
		{
			name:          "mixed complete and partial",
			input:         append([]byte("ok✅"), 0xE2, 0x9C),
			wantComplete:  "ok✅",
			wantRemainLen: 2,
		},
		{
			name:          "empty input",
			input:         []byte{},
			wantComplete:  "",
			wantRemainLen: 0,
		},
		{
			name:          "lone continuation byte passes through",
			input:         []byte{0x80},
			wantComplete:  string([]byte{0x80}),
			wantRemainLen: 0,
		},
		{
			name:          "lone continuation byte does not block valid content",
			input:         []byte{0x80, 'H', 'i'},
			wantComplete:  string([]byte{0x80, 'H', 'i'}),
			wantRemainLen: 0,
		},
		{
			name:          "invalid byte FF passes through",
			input:         []byte{0xFF, 'o', 'k'},
			wantComplete:  string([]byte{0xFF, 'o', 'k'}),
			wantRemainLen: 0,
		},
		{
			name:          "2-byte sequence complete",
			input:         []byte{0xC3, 0xA9},
			wantComplete:  "é",
			wantRemainLen: 0,
		},
		{
			name:          "2-byte sequence partial (leading byte only)",
			input:         []byte{0xC3},
			wantComplete:  "",
			wantRemainLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			complete, remainder := extractCompleteUTF8(tt.input)

			if string(complete) != tt.wantComplete {
				t.Errorf("complete: got %q (%x), want %q (%x)",
					string(complete), complete, tt.wantComplete, []byte(tt.wantComplete))
			}

			if len(remainder) != tt.wantRemainLen {
				t.Errorf("remainder length: got %d, want %d", len(remainder), tt.wantRemainLen)
			}
		})
	}
}

func TestExtractCompleteUTF8_Reassembly(t *testing.T) {
	fire := []byte("🔥")
	chunk1 := fire[:2]
	chunk2 := fire[2:]

	complete1, remainder1 := extractCompleteUTF8(chunk1)
	if len(complete1) != 0 {
		t.Fatalf("chunk1 should produce no complete output, got %q", complete1)
	}

	combined := append(remainder1, chunk2...)
	complete2, remainder2 := extractCompleteUTF8(combined)

	if string(complete2) != "🔥" {
		t.Errorf("reassembled: got %q, want 🔥", string(complete2))
	}
	if len(remainder2) != 0 {
		t.Errorf("remainder after reassembly: got %d bytes, want 0", len(remainder2))
	}
}

func TestExtractCompleteUTF8_ThreeChunkReassembly(t *testing.T) {
	fire := []byte("🔥")

	var buf []byte
	for _, b := range fire {
		buf = append(buf, b)
		complete, remainder := extractCompleteUTF8(buf)
		buf = remainder

		if len(buf) > 0 && len(complete) > 0 {
			t.Errorf("got both complete=%q and remainder with byte-by-byte feed", complete)
		}

		if len(complete) > 0 {
			if string(complete) != "🔥" {
				t.Errorf("final reassembly: got %q, want 🔥", string(complete))
			}
		}
	}

	if len(buf) != 0 {
		t.Errorf("buffer should be empty after all bytes, got %d bytes", len(buf))
	}
}
