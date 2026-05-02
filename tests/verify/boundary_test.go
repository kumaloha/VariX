package verify_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestVerifyDoesNotImportCompile(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", ".")
	cmd.Dir = "."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list verify package: %v", err)
	}
	var pkg struct {
		Imports []string
	}
	if err := json.Unmarshal(out, &pkg); err != nil {
		t.Fatalf("parse go list output: %v", err)
	}
	for _, path := range pkg.Imports {
		if strings.TrimSpace(path) == "github.com/kumaloha/VariX/varix/compile" {
			t.Fatalf("verify must depend on analysis, not compile")
		}
	}
}
