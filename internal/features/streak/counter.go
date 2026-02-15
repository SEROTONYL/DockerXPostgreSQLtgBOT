// Package streak — counter.go отвечает за подсчёт слов в сообщениях.
// Сообщение засчитывается для стрика только если содержит 3+ слов.
package streak

import "strings"

// CountWords подсчитывает количество слов в тексте.
// Слова разделяются пробелами (включая множественные пробелы, табы и т.д.).
//
// Примеры:
//
//	CountWords("привет как дела") → 3 (засчитывается)
//	CountWords("ок")              → 1 (НЕ засчитывается)
//	CountWords("  пробелы  лишние  ") → 2 (НЕ засчитывается)
func CountWords(text string) int {
	// strings.Fields разбивает строку по пробельным символам
	// и автоматически игнорирует лишние пробелы
	words := strings.Fields(text)
	return len(words)
}

// IsValidForStreak проверяет, подходит ли сообщение для подсчёта стрика.
// Условия:
//   - Минимум 3 слова
//   - Не является командой (не начинается с !, . или /)
func IsValidForStreak(text string) bool {
	text = strings.TrimSpace(text)

	// Игнорируем команды
	if strings.HasPrefix(text, "!") || strings.HasPrefix(text, ".") || strings.HasPrefix(text, "/") {
		return false
	}

	// Проверяем количество слов
	return CountWords(text) >= 3
}
