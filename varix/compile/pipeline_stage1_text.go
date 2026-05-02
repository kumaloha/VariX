package compile

import (
	"strings"
)

func containsAnyText(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
