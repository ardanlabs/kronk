package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLifecycleEvidenceUsesOnlyAppendedMatchingEvents(t *testing.T) {
	dir := t.TempDir()
	serverLog := filepath.Join(dir, "kronk.log")
	if err := os.WriteFile(serverLog, []byte(`{"msg":"old","trace_id":"11111111111111111111111111111111"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts, err := newLifecycleArtifacts(filepath.Join(dir, "output"), serverLog)
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.close()
	file, err := os.OpenFile(serverLog, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"msg":"request-lifecycle","trace_id":"44444444444444444444444444444444"}` + "\n" +
		`{"msg":"request-lifecycle","trace_id":"ffffffffffffffffffffffffffffffff"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	evidence := artifacts.collectServerEvidence()
	if evidence.MatchedEvents != 1 {
		t.Fatalf("matched events = %d, want 1", evidence.MatchedEvents)
	}
}
