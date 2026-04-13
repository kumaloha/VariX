package config

import (
	"path/filepath"

	"github.com/kumaloha/forge/llm"
)

// NewPromptLoader creates a project-scoped prompt loader rooted at projectRoot/prompts.
func NewPromptLoader(projectRoot string) *llm.PromptLoader {
	return llm.NewPromptLoaderFromDir(filepath.Join(projectRoot, "prompts"))
}
