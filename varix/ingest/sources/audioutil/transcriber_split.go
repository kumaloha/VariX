package audioutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

func splitWithFFmpeg(ctx context.Context, inputPath string, seconds int) (splitArtifacts, error) {
	splitDir, err := os.MkdirTemp("", "invarix-audio-segments-*")
	if err != nil {
		return splitArtifacts{}, err
	}
	pattern := filepath.Join(splitDir, "segment-%03d.mp3")
	if err := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-i", inputPath,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", seconds),
		"-c", "copy",
		pattern,
	).Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return splitArtifacts{CleanupDir: splitDir}, fmt.Errorf("tool missing: ffmpeg: %w", err)
		}
		return splitArtifacts{CleanupDir: splitDir}, err
	}

	parts, err := filepath.Glob(filepath.Join(splitDir, "segment-*.mp3"))
	if err != nil {
		return splitArtifacts{CleanupDir: splitDir}, err
	}
	if len(parts) == 0 {
		return splitArtifacts{CleanupDir: splitDir}, fmt.Errorf("audio split produced no segments")
	}
	sort.Strings(parts)
	return splitArtifacts{Parts: parts, CleanupDir: splitDir}, nil
}

// Download fetches a remote video URL to a temp file with size limits.
// The caller is responsible for removing the returned file.
