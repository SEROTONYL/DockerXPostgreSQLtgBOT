// Package common содержит общие утилиты, используемые во всём проекте.
// Сюда входят: русская плюрализация, форматирование чисел, работа с временем.
package common

import (
	"fmt"
	"math"
	"time"
)

// PluralizeFilms возвращает правильную форму слова «пленка» для числа n.
//
// Правила русского языка:
//   - n%10==1 И n%100!=11 → "пленка" (1, 21, 31, 101, ...)
//   - n%10 в [2,3,4] И n%100 НЕ в [12,13,14] → "пленки" (2, 3, 4, 22, 23, ...)
//   - Остальные случаи → "пленок" (0, 5-20, 25-30, 100, ...)
//
// Примеры:
//
//	PluralizeFilms(1)  → "пленка"
//	PluralizeFilms(3)  → "пленки"
//	PluralizeFilms(5)  → "пленок"
//	PluralizeFilms(11) → "пленок"
//	PluralizeFilms(21) → "пленка"
func PluralizeFilms(n int64) string {
	// Берём абсолютное значение для отрицательных чисел
	absN := int64(math.Abs(float64(n)))
	lastDigit := absN % 10
	lastTwoDigits := absN % 100

	// Единственное число: 1, 21, 31, 101 (но НЕ 11, 111)
	if lastDigit == 1 && lastTwoDigits != 11 {
		return "пленка"
	}

	// Малое множественное: 2-4, 22-24, 32-34 (но НЕ 12-14)
	if lastDigit >= 2 && lastDigit <= 4 && (lastTwoDigits < 12 || lastTwoDigits > 14) {
		return "пленки"
	}

	// Большое множественное: 0, 5-20, 25-30, 100, ...
	return "пленок"
}

// FormatBalance форматирует баланс в читабельную строку.
// Пример: FormatBalance(150) → "150 пленок"
func FormatBalance(balance int64) string {
	return fmt.Sprintf("%d %s", balance, PluralizeFilms(balance))
}

// PluralizeDays возвращает правильную форму слова «день» для числа n.
//
// Правила:
//   - 1, 21, 31 → "день"
//   - 2-4, 22-24 → "дня"
//   - 5-20, 25-30 → "дней"
func PluralizeDays(n int) string {
	absN := int(math.Abs(float64(n)))
	lastDigit := absN % 10
	lastTwoDigits := absN % 100

	if lastDigit == 1 && lastTwoDigits != 11 {
		return "день"
	}
	if lastDigit >= 2 && lastDigit <= 4 && (lastTwoDigits < 12 || lastTwoDigits > 14) {
		return "дня"
	}
	return "дней"
}

// PluralizeMessages возвращает правильную форму слова «сообщение».
func PluralizeMessages(n int) string {
	absN := int(math.Abs(float64(n)))
	lastDigit := absN % 10
	lastTwoDigits := absN % 100

	if lastDigit == 1 && lastTwoDigits != 11 {
		return "сообщение"
	}
	if lastDigit >= 2 && lastDigit <= 4 && (lastTwoDigits < 12 || lastTwoDigits > 14) {
		return "сообщения"
	}
	return "сообщений"
}

// GetMoscowTime возвращает текущее время в часовом поясе Москвы (Europe/Moscow).
// Используется для ежедневного сброса стриков в полночь по Москве.
func GetMoscowTime() time.Time {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		// Если не удалось загрузить — используем UTC+3 вручную
		loc = time.FixedZone("MSK", 3*60*60)
	}
	return time.Now().In(loc)
}

// GetMoscowDate возвращает только дату (без времени) в часовом поясе Москвы.
// Формат: 2006-01-02
func GetMoscowDate() time.Time {
	t := GetMoscowTime()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// FormatDateTime форматирует время в формат "02.01.2006 15:04" (день.месяц.год часы:минуты).
// Используется для отображения дат транзакций.
func FormatDateTime(t time.Time) string {
	loc, _ := time.LoadLocation("Europe/Moscow")
	return t.In(loc).Format("02.01.2006 15:04")
}
