package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	description := normalize.CollapseWhitespace(payload.Description)
	return Metadata{
		Title:       payload.Title,
		Uploader:    author,
		Description: description,
		SourceLinks: normalize.ExtractURLs(description),
		PublishedAt: postedAt,
	}, nil
}
