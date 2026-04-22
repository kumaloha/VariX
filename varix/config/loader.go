package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func Get(root, key string) (string, bool) {
	if value, ok := os.LookupEnv(key); ok {
		return value, true
	}

	for _, name := range []string{".env.local", ".env"} {
		file, err := os.Open(filepath.Join(root, name))
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		prefix := key + "="
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, prefix) {
				return parseDotEnvValue(strings.TrimSpace(strings.TrimPrefix(line, prefix))), true
			}
		}
	}
	return "", false
}

func FirstConfiguredValue(root string, keys ...string) string {
	for _, key := range keys {
		if value, ok := Get(root, key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseDotEnvValue(raw string) string {
	value := strings.TrimSpace(raw)
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = value[:idx]
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
