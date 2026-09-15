package prepare

import (
	"fmt"
	"strings"
	"testing"
)

func TestPreviewPassagesSamplesCompleteSubstantiveParagraphs(t *testing.T) {
	paragraphs := make([]string, 30)
	for index := range paragraphs {
		paragraphs[index] = fmt.Sprintf("Paragraph %d contains a complete supported fact for training.", index)
	}
	source := "Title\n\n" + strings.Join(paragraphs, "\n\n") + "\n\nIndex"

	passages := previewPassages(source)
	if len(passages) != previewPassageLimit {
		t.Fatalf("preview passages = %d, want %d", len(passages), previewPassageLimit)
	}
	for _, passage := range passages {
		if !strings.Contains(source, passage) || passage == "Title" || passage == "Index" {
			t.Fatalf("preview included a cut or insubstantial passage: %q", passage)
		}
	}
}

func TestPreviewPassagesKeepsAllUsableShortDocumentParagraphs(t *testing.T) {
	source := "Alpha contains the first supported fact.\n\nBeta contains the second supported fact.\n\nGamma contains the third supported fact."
	if passages := previewPassages(source); len(passages) != 3 {
		t.Fatalf("preview passages = %d, want all three short-document paragraphs", len(passages))
	}
}

func TestFullPassagesCombinesSmallParagraphsIntoBoundedEvidence(t *testing.T) {
	var source strings.Builder
	for index := range 1000 {
		fmt.Fprintf(&source, "Grounded statement %d remains available for complete training coverage.\n\n", index)
	}

	passages := fullPassages(source.String(), nil)
	batches := fullPassageBatches(passages)
	if len(passages) > 25 {
		t.Fatalf("evidence passages = %d, want at most 25", len(passages))
	}
	if len(batches) > 5 {
		t.Fatalf("model batches = %d, want at most 5", len(batches))
	}
	for index, passage := range passages {
		if len([]rune(passage)) > fullEvidenceRunes {
			t.Fatalf("passage %d has %d runes, maximum %d", index, len([]rune(passage)), fullEvidenceRunes)
		}
	}
}
