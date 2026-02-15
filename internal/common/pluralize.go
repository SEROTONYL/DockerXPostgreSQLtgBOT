// Package common — pluralize.go содержит вспомогательные функции
// для правильного склонения русских числительных.
// Основная логика плюрализации реализована в helpers.go,
// этот файл экспортирует дополнительные утилиты.
package common

import "fmt"

// FormatFilmsAmount создаёт строку вида "+100 пленок" или "-50 пленок".
// Знак «+» или «-» добавляется автоматически.
//
// Примеры:
//
//	FormatFilmsAmount(100)  → "+100 пленок"
//	FormatFilmsAmount(-50)  → "-50 пленок"
//	FormatFilmsAmount(1)    → "+1 пленка"
func FormatFilmsAmount(amount int64) string {
	if amount >= 0 {
		return fmt.Sprintf("+%d %s", amount, PluralizeFilms(amount))
	}
	return fmt.Sprintf("%d %s", amount, PluralizeFilms(amount))
}

// FormatNumber форматирует число с разделителями тысяч (пробелами).
// Пример: FormatNumber(2350) → "2 350"
func FormatNumber(n int64) string {
	// Простая реализация для чисел до миллиарда
	if n < 0 {
		return "-" + FormatNumber(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	// Рекурсивно добавляем разделители
	rest := n / 1000
	last := n % 1000

	if rest > 0 {
		return fmt.Sprintf("%s %03d", FormatNumber(rest), last)
	}
	return fmt.Sprintf("%d", last)
}
