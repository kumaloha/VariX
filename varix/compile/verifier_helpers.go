package compile

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func truthy(value any) bool {
	if v, ok := value.(bool); ok {
		return v
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
