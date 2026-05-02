package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseCompilePayload(raw string) (map[string]json.RawMessage, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)
	clean = extractFirstJSONObject(clean)

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(clean), &payload); err != nil {
		repaired := repairSuspiciousQuotesInJSONObject(clean)
		if repaired != clean {
			if retryErr := json.Unmarshal([]byte(repaired), &payload); retryErr == nil {
				return payload, nil
			}
		}
		return nil, fmt.Errorf("parse compile output: %w", err)
	}
	return payload, nil
}
func repairSuspiciousQuotesInJSONObject(raw string) string {
	if raw == "" {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if !inString {
			b.WriteByte(ch)
			if ch == '"' {
				inString = true
			}
			continue
		}
		if escaped {
			b.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			b.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			next := nextNonSpaceByte(raw, i+1)
			if next == ',' || next == '}' || next == ']' || next == ':' || next == 0 {
				b.WriteByte(ch)
				inString = false
				continue
			}
			b.WriteRune('”')
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
func nextNonSpaceByte(raw string, start int) byte {
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return raw[i]
		}
	}
	return 0
}
func extractFirstJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return raw
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(raw[start : i+1])
			}
		}
	}
	return raw
}
