package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/normalize"
)

type YTDLPMetadataFetcher struct{}

func NewYTDLPMetadataFetcher() *YTDLPMetadataFetcher {
	return &YTDLPMetadataFetcher{}
}

func (f *YTDLPMetadataFetcher) Fetch(ctx context.Context, videoID string) (Metadata, error) {
	cmd := exec.CommandContext(
		ctx,
		"yt-dlp",
		"--dump-single-json",
		"--no-playlist",
		"https://www.bilibili.com/video/"+videoID,
	)
	output, err := cmd.Output()
	if err != nil {
		return Metadata{}, err
	}

	var payload struct {
		Title       string `json:"title"`
		Uploader    string `json:"uploader"`
		Channel     string `json:"channel"`
		Description string `json:"description"`
		Timestamp   int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return Metadata{}, err
	}
	if payload.Title == "" {
		return Metadata{}, fmt.Errorf("bilibili metadata title missing for %s", videoID)
	}

	postedAt := time.Time{}
	if payload.Timestamp > 0 {
		postedAt = time.Unix(payload.Timestamp, 0).UTC()
	}
	author := payload.Uploader
	if author == "" {
		author = payload.Channel
	}
	rawDescription := payload.Description
	description := normalize.CollapseWhitespace(rawDescription)
	return Metadata{
		Title:       payload.Title,
		Uploader:    author,
		Description: description,
		SourceLinks: extractVideoSourceLinks(rawDescription),
		PublishedAt: postedAt,
	}, nil
}

func extractVideoSourceLinks(description string) []string {
	lines := strings.Split(description, "\n")
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !looksLikeSourceLine(line) {
			continue
		}
		for _, link := range normalize.ExtractURLs(line) {
			if _, ok := seen[link]; ok {
				continue
			}
			seen[link] = struct{}{}
			out = append(out, link)
		}
	}
	return out
}

func looksLikeSourceLine(line string) bool {
	clean := strings.TrimSpace(line)
	for _, url := range normalize.ExtractURLs(clean) {
		clean = strings.ReplaceAll(clean, url, "")
	}
	lower := strings.ToLower(normalize.CollapseWhitespace(clean))
	markers := []string{
		"原视频", "原影片", "原視頻", "原片", "原文", "原帖", "原始链接", "原始連結", "來源", "来源", "資料來源", "资料来源",
		"source", "original video", "original post", "original article", "full video", "full interview", "reference",
	}
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
