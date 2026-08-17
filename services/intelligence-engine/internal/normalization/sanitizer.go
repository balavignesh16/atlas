package normalization

import (
	"strings"
)

// IsSensitiveAttribute returns true if the key contains sensitive substrings
func IsSensitiveAttribute(key string) bool {
	lowerKey := strings.ToLower(key)
	sensitiveTokens := []string{
		"authorization",
		"password",
		"token",
		"secret",
		"api_key",
		"credential",
	}

	for _, token := range sensitiveTokens {
		if strings.Contains(lowerKey, token) {
			return true
		}
	}
	return false
}

// SanitizeString truncates strings that are too long to prevent memory exhaustion
func SanitizeString(val string, limit int) string {
	if len(val) > limit {
		return val[:limit] + "...(truncated)"
	}
	return val
}
