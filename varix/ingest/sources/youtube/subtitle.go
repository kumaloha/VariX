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

var (
	vttNumber                 = regexp.MustCompile(`^\d+$`)
	preferredSubtitleLangsArg = "en,zh-CN,zh-TW,zh-Hans,zh-Hant,zh"
	fallbackSubtitleLangsArg  = "all,-live_chat"
	preferredSubtitleTags     = []string{"en", "en-us", "en-gb", "en-au", "zh-cn", "zh-tw", "zh-hans", "zh-hant", "zh"}
)

func (f *YTDLPSubtitleFetcher) Fetch(ctx context.Context, videoID string) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "invarix-yt-subs-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	output := filepath.Join(tmpDir, videoID)
	matches, err := downloadSubtitleFiles(ctx, videoID, output, preferredSubtitleLangsArg)
	if err != nil && len(matches) == 0 {
		if errors.Is(err, exec.ErrNotFound) {
			return "", "", fmt.Errorf("tool missing: yt-dlp: %w", err)
		}
		return "", "", err
	}
	if len(matches) == 0 {
		matches, err = downloadSubtitleFiles(ctx, videoID, output, fallbackSubtitleLangsArg)
		if err != nil && len(matches) == 0 {
			if errors.Is(err, exec.ErrNotFound) {
				return "", "", fmt.Errorf("tool missing: yt-dlp: %w", err)
			}
			return "", "", err
		}
	}
	if len(matches) == 0 {
		return "", "", nil
	}
	path := preferredSubtitleFile(matches)
	if path == "" {
		return "", "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	lines := strings.Split(string(data), "\n")
	textLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "WEBVTT") || strings.HasPrefix(line, "Kind:") || strings.HasPrefix(line, "Language:") || strings.Contains(line, "-->") || vttNumber.MatchString(line) {
			continue
		}
		textLines = append(textLines, line)
	}
	if len(textLines) == 0 {
		return "", "", nil
	}
	return strings.Join(textLines, " "), "subtitle_vtt", nil
}

func downloadSubtitleFiles(ctx context.Context, videoID, output, langArg string) ([]string, error) {
	cmd := exec.CommandContext(
		ctx,
		"yt-dlp",
		"--skip-download",
		"--write-auto-subs",
		"--write-subs",
		"--sub-langs", langArg,
		"--sub-format", "vtt",
		"-o", output,
		"https://www.youtube.com/watch?v="+videoID,
	)
	err := cmd.Run()
	matches, globErr := filepath.Glob(output + "*.vtt")
	if globErr != nil {
		return nil, globErr
	}
	return matches, err
}

func preferredSubtitleFile(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	best := matches[0]
	bestScore := subtitleFilePriority(best)
	for _, match := range matches[1:] {
		score := subtitleFilePriority(match)
		if score < bestScore || (score == bestScore && strings.ToLower(match) < strings.ToLower(best)) {
			best = match
			bestScore = score
		}
	}
	return best
}

func subtitleFilePriority(path string) int {
	tag := subtitleLanguageTag(path)
	if tag == "" {
		return 100
	}
	lower := strings.ToLower(tag)
	for i, preferred := range preferredSubtitleTags {
		if lower == preferred {
			return i
		}
	}
	if isLikelyTranslatedSubtitleTag(lower) {
		return 90
	}
	return 50
}

func subtitleLanguageTag(path string) string {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".vtt")
	idx := strings.LastIndex(name, ".")
	if idx < 0 || idx == len(name)-1 {
		return ""
	}
	return name[idx+1:]
}

func isLikelyTranslatedSubtitleTag(tag string) bool {
	parts := strings.Split(tag, "-")
	return len(parts) >= 3
}
