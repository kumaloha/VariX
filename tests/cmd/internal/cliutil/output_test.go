package cliutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePayloadStdoutOnly(t *testing.T) {
	var stdout bytes.Buffer
	if err := WritePayload([]byte("hello"), "", "", &stdout); err != nil {
		t.Fatalf("WritePayload() error = %v", err)
	}
	if stdout.String() != "hello" {
		t.Fatalf("stdout = %q, want hello", stdout.String())
	}
}

func TestWritePayloadWritesPrimaryAndLatest(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.txt")
	latest := filepath.Join(dir, "latest.txt")
	var stdout bytes.Buffer

	if err := WritePayload([]byte("payload"), primary, latest, &stdout); err != nil {
		t.Fatalf("WritePayload() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when primary file output is used", stdout.String())
	}
	gotPrimary, err := os.ReadFile(primary)
	if err != nil {
		t.Fatalf("ReadFile(primary) error = %v", err)
	}
	gotLatest, err := os.ReadFile(latest)
	if err != nil {
		t.Fatalf("ReadFile(latest) error = %v", err)
	}
	if string(gotPrimary) != "payload" || string(gotLatest) != "payload" {
		t.Fatalf("primary=%q latest=%q, want both payload", string(gotPrimary), string(gotLatest))
	}
}
