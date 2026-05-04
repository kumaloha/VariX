package compile

import "strings"

func leadTitleFromBundle(bundle Bundle) string {
	return leadTitleFromText(bundle.Content)
}

func leadTitleFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "【") {
		if before, _, ok := strings.Cut(strings.TrimPrefix(text, "【"), "】"); ok {
			return truncateRunes(strings.TrimSpace(before), 48)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		line = strings.TrimSpace(strings.TrimRight(line, "。.!！"))
		if line == "" {
			continue
		}
		if len([]rune(line)) <= 48 {
			return line
		}
		return ""
	}
	return ""
}
