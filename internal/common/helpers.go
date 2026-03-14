package common

import (
	"fmt"
	"math"
	"time"
)

const FilmFramesEmoji = "🎞️"

// PluralizeFilms keeps the historical API, but currency is now rendered as a
// single emoji marker in user-facing text.
func PluralizeFilms(n int64) string {
	_ = n
	return FilmFramesEmoji
}

func FormatBalance(balance int64) string {
	return fmt.Sprintf("%d%s", balance, FilmFramesEmoji)
}

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

func GetMoscowTime() time.Time {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*60*60)
	}
	return time.Now().In(loc)
}

func GetMoscowDate() time.Time {
	t := GetMoscowTime()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func FormatDateTime(t time.Time) string {
	loc, _ := time.LoadLocation("Europe/Moscow")
	return t.In(loc).Format("02.01.2006 15:04")
}
