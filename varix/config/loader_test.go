package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetPrefersProcessEnvOverDotEnvLocal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("TWITTER_BEARER_TOKEN=file-token\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("TWITTER_BEARER_TOKEN", "process-token")

	got, ok := Get(root, "TWITTER_BEARER_TOKEN")
	if !ok {
		t.Fatal("expected key to be found")
	}
	if got != "process-token" {
		t.Fatalf("Get() = %q, want %q", got, "process-token")
	}
}

func TestGet_EmptyProcessEnvStillShadowsDotEnvLocal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("TWITTER_BEARER_TOKEN=file-token\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("TWITTER_BEARER_TOKEN", "")

	got, ok := Get(root, "TWITTER_BEARER_TOKEN")
	if !ok {
		t.Fatal("expected key to be found from process env")
	}
	if got != "" {
		t.Fatalf("Get() = %q, want empty string", got)
	}
}

func TestGet_StripsQuotesFromDotEnvLocalValue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("OPENAI_API_KEY=\"sk-abc\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok := Get(root, "OPENAI_API_KEY")
	if !ok {
		t.Fatal("expected key to be found")
	}
	if got != "sk-abc" {
		t.Fatalf("Get() = %q, want %q", got, "sk-abc")
	}
}

func TestGet_StripsTrailingCommentFromDotEnvLocalValue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("OPENAI_API_KEY=sk-abc # prod\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok := Get(root, "OPENAI_API_KEY")
	if !ok {
		t.Fatal("expected key to be found")
	}
	if got != "sk-abc" {
		t.Fatalf("Get() = %q, want %q", got, "sk-abc")
	}
}

func TestGet_StripsTrailingCommentAfterQuotedDotEnvLocalValue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("OPENAI_API_KEY=\"sk-abc\" # prod\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok := Get(root, "OPENAI_API_KEY")
	if !ok {
		t.Fatal("expected key to be found")
	}
	if got != "sk-abc" {
		t.Fatalf("Get() = %q, want %q", got, "sk-abc")
	}
}

func TestGet_FallsBackToDotEnvWhenDotEnvLocalMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("ASR_API_KEY=env-token\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok := Get(root, "ASR_API_KEY")
	if !ok {
		t.Fatal("expected key to be found in .env")
	}
	if got != "env-token" {
		t.Fatalf("Get() = %q, want %q", got, "env-token")
	}
}

func TestGet_PrefersDotEnvLocalOverDotEnv(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("ASR_API_KEY=env-token\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("ASR_API_KEY=local-token\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok := Get(root, "ASR_API_KEY")
	if !ok {
		t.Fatal("expected key to be found")
	}
	if got != "local-token" {
		t.Fatalf("Get() = %q, want %q", got, "local-token")
	}
}
