package config

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Settings struct {
	ProjectRoot            string
	ConfigDir              string
	PromptsDir             string
	ContentDir             string
	ContentDBPath          string
	StoreBackend           string
	PollInterval           time.Duration
	ReuseStoredTranscripts bool
}

func DefaultSettings(projectRoot string) Settings {
	settings := Settings{
		ProjectRoot:            projectRoot,
		ConfigDir:              filepath.Join(projectRoot, "config"),
		PromptsDir:             filepath.Join(projectRoot, "prompts"),
		ContentDir:             filepath.Join(projectRoot, "data", "content"),
		ContentDBPath:          filepath.Join(projectRoot, "data", "content.db"),
		StoreBackend:           "sqlite",
		PollInterval:           15 * time.Minute,
		ReuseStoredTranscripts: true,
	}
	if backend, ok := Get(projectRoot, "INVARIX_STORE_BACKEND"); ok && strings.TrimSpace(backend) != "" {
		settings.StoreBackend = strings.TrimSpace(backend)
	}
	if dbPath, ok := Get(projectRoot, "INVARIX_CONTENT_DB_PATH"); ok && strings.TrimSpace(dbPath) != "" {
		settings.ContentDBPath = strings.TrimSpace(dbPath)
	}
	if value, ok := Get(projectRoot, "INVARIX_REUSE_STORED_TRANSCRIPTS"); ok {
		if parsed, ok := parseBool(value); ok {
			settings.ReuseStoredTranscripts = parsed
		}
	}
	return settings
}

func parseBool(raw string) (bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, false
	}
	parsed, err := strconv.ParseBool(trimmed)
	if err == nil {
		return parsed, true
	}
	switch strings.ToLower(trimmed) {
	case "on", "yes":
		return true, true
	case "off", "no":
		return false, true
	default:
		return false, false
	}
}
