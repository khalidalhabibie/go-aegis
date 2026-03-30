package webhooks

import "strings"

func sanitizeResponseBody(body []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 512
	}

	truncated := false
	if len(body) > maxBytes {
		body = body[:maxBytes]
		truncated = true
	}

	sanitized := strings.ToValidUTF8(string(body), "\uFFFD")
	sanitized = strings.ReplaceAll(sanitized, "\x00", "")

	if truncated {
		return sanitized + "...[truncated]"
	}

	return sanitized
}
