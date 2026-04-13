package config

import (
	"path/filepath"
	"strings"
	"time"
)

type Settings struct {
	ProjectRoot     string
	ConfigDir       string
	PromptsDir      string
	ContentDir      string
	ContentDBPath   string
	StoreBackend    string
	ProvenanceJudge string
	PollInterval    time.Duration
}

func DefaultSettings(projectRoot string) Settings {
	settings := Settings{
		ProjectRoot:     projectRoot,
		ConfigDir:       filepath.Join(projectRoot, "config"),
		PromptsDir:      filepath.Join(projectRoot, "prompts"),
		ContentDir:      filepath.Join(projectRoot, "data", "content"),
		ContentDBPath:   filepath.Join(projectRoot, "data", "content.db"),
		StoreBackend:    "sqlite",
		ProvenanceJudge: "deterministic",
		PollInterval:    15 * time.Minute,
	}
	if backend, ok := Get(projectRoot, "INVARIX_STORE_BACKEND"); ok && strings.TrimSpace(backend) != "" {
		settings.StoreBackend = strings.TrimSpace(backend)
	}
	if dbPath, ok := Get(projectRoot, "INVARIX_CONTENT_DB_PATH"); ok && strings.TrimSpace(dbPath) != "" {
		settings.ContentDBPath = strings.TrimSpace(dbPath)
	}
	if judge, ok := Get(projectRoot, "VARIX_PROVENANCE_JUDGE"); ok && strings.TrimSpace(judge) != "" {
		settings.ProvenanceJudge = strings.TrimSpace(judge)
	}
	return settings
}
