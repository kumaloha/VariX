package bilibili

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractVideoSourceLinks_FiltersPromoLinks(t *testing.T) {
	description := "加入会员：https://space.bilibili.com/1\n原视频：https://www.example.com/original\n商品链接：https://shop.example.com/a"
	got := extractVideoSourceLinks(description)
	if len(got) != 1 {
		t.Fatalf("len(SourceLinks) = %d, want 1 (%#v)", len(got), got)
	}
	if got[0] != "https://www.example.com/original" {
		t.Fatalf("SourceLinks[0] = %q, want original source", got[0])
	}
}

func TestYTDLPMetadataFetcher_FiltersDescriptionSourceLinks(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "yt-dlp")
	payload, _ := json.Marshal(map[string]any{
		"title":       "sample",
		"uploader":    "uploader",
		"description": "加入会员：https://space.bilibili.com/1\n原视频：https://www.example.com/original",
		"timestamp":   1712554979,
	})
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' '"+string(payload)+"'\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath)
	got, err := NewYTDLPMetadataFetcher().Fetch(context.Background(), "BV1")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got.SourceLinks) != 1 || got.SourceLinks[0] != "https://www.example.com/original" {
		t.Fatalf("SourceLinks = %#v", got.SourceLinks)
	}
}
