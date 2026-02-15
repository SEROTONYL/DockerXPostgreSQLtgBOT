// Package karma — detector.go определяет, содержит ли сообщение «спасибо».
package karma

import "strings"

// IsThankYou проверяет, является ли текст «спасибо».
// Регистр не важен. Пунктуация в конце допускается.
func IsThankYou(text string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(text))
	cleaned = strings.TrimRight(cleaned, "!.,;:)")
	return cleaned == "спасибо"
}
