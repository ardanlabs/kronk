package downapp

import (
	"strings"
	"testing"
)

func TestPeerPullEventEndsWithBlankLine(t *testing.T) {
	event := toPeerPullEvent(PeerPullEvent{Status: "downloading"})
	if !strings.HasSuffix(event, "\n\n") {
		t.Errorf("event: got %q, want blank-line terminator", event)
	}
}
