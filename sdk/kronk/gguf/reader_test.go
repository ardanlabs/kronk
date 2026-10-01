package gguf

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestParseMetadataRejectsTruncatedMetadata(t *testing.T) {
	var data bytes.Buffer
	header := Header{
		Magic:           Magic,
		Version:         Version,
		MetadataKvCount: 1,
	}
	if err := binary.Write(&data, binary.LittleEndian, header); err != nil {
		t.Fatalf("write header: %v", err)
	}

	metadata, err := ParseMetadata(data.Bytes())
	if err == nil || !strings.Contains(err.Error(), "read metadata entry 0") {
		t.Fatalf("ParseMetadata: got metadata %v and error %v, want truncated metadata error", metadata, err)
	}
}
