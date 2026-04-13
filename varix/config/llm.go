package config

import (
	"path/filepath"

	"github.com/kumaloha/forge/llm"
)

type llmEngineParams struct {
	LLM llm.LLMConfig `yaml:"llm"`
}

// LoadLLMConfig reads config/engine_params.yaml and returns the validated llm section.
func LoadLLMConfig(projectRoot string) (llm.LLMConfig, error) {
	params, err := LoadYAML[llmEngineParams](projectRoot, filepath.Join("config", "engine_params.yaml"))
	if err != nil {
		return llm.LLMConfig{}, err
	}
	if err := params.LLM.Validate(); err != nil {
		return llm.LLMConfig{}, err
	}
	return params.LLM, nil
}
