package youtube

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type YTDLPSubtitleFetcher struct{}

func NewYTDLPSubtitleFetcher() *YTDLPSubtitleFetcher {
	return &YTDLPSubtitleFetcher{}
}

var vttNumber = regexp.MustCompile(`^\d+$`)

func (f *YTDLPSubtitleFetcher) Fetch(ctx context.Context, videoID string) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "invarix-yt-subs-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	output := filepath.Join(tmpDir, videoID)
	cmd := exec.CommandContext(
		ctx,
		"yt-dlp",
		"--skip-download",
		"--write-auto-subs",
		"--write-subs",
		"--sub-langs", "zh-Hans,zh-Hant,zh,en",
		"--sub-format", "vtt",
		"-o", output,
		"https://www.youtube.com/watch?v="+videoID,
	)
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", "", fmt.Errorf("tool missing: yt-dlp: %w", err)
		}
		return "", "", err
	}

	matches, err := filepath.Glob(output + "*.vtt")
	if err != nil {
		return "", "", err
	}
	if len(matches) == 0 {
		return "", "", nil
	}
	sort.Strings(matches)

	data, err := os.ReadFile(matches[0])
	if err != nil {
		return "", "", err
	}

	lines := strings.Split(string(data), "\n")
	textLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "WEBVTT") || strings.Contains(line, "-->") || vttNumber.MatchString(line) {
			continue
		}
		textLines = append(textLines, line)
	}
	if len(textLines) == 0 {
		return "", "", nil
	}
	return strings.Join(textLines, " "), "subtitle_vtt", nil
}
