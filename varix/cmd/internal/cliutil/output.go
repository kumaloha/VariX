package cliutil

import (
	"io"
	"os"
)

// WritePayload writes payload to optional primary/latest files or stdout.
func WritePayload(payload []byte, outPath, latestOutPath string, stdout io.Writer) error {
	if outPath != "" {
		if err := os.WriteFile(outPath, payload, 0o644); err != nil {
			return err
		}
	}
	if latestOutPath != "" {
		if err := os.WriteFile(latestOutPath, payload, 0o644); err != nil {
			return err
		}
	}
	if outPath != "" {
		return nil
	}
	_, err := stdout.Write(payload)
	return err
}
