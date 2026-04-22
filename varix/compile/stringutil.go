package compile

import "strings"

func CloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
