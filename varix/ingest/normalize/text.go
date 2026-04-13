package normalize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var urlRE = regexp.MustCompile(`https?://[^\s<>"'\])]+`)

func CollapseWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func JoinParagraphs(lines []string) string {
	paragraphs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = CollapseWhitespace(line)
		if line == "" {
			continue
		}
		paragraphs = append(paragraphs, line)
	}
	return strings.Join(paragraphs, "\n\n")
}

func ExtractURLs(value string) []string {
	matches := urlRE.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		clean := trimTrailingURLNoise(match)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func trimTrailingURLNoise(value string) string {
	for len(value) > 0 {
		r, size := utf8.DecodeLastRuneInString(value)
		switch {
		case unicode.IsSpace(r), unicode.IsPunct(r), unicode.IsSymbol(r):
			value = value[:len(value)-size]
			continue
		case unicode.IsLetter(r) && r > unicode.MaxASCII:
			value = value[:len(value)-size]
			continue
		default:
			return value
		}
	}
	return value
}
