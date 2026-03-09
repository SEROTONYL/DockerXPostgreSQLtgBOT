package streak

import (
	"strings"
	"unicode"
)

func CountWords(text string) int {
	return len(strings.Fields(text))
}

func IsIgnoredCommand(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "/")
}

func IsEmojiOnly(text string) bool {
	hasVisible := false
	hasLetterOrDigit := false
	for _, r := range strings.TrimSpace(text) {
		if unicode.IsSpace(r) {
			continue
		}
		hasVisible = true
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasLetterOrDigit = true
			break
		}
	}
	return hasVisible && !hasLetterOrDigit
}

func normalizeMessageText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func IsValidForStreak(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if IsIgnoredCommand(text) {
		return false
	}
	if CountWords(text) <= 4 {
		return false
	}
	if IsEmojiOnly(text) {
		return false
	}
	return true
}
