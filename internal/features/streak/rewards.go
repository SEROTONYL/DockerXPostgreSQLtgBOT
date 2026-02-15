// Package streak — rewards.go содержит логику расчёта бонусов за стрики.
// Бонусы начисляются МОЛЧА (без уведомления пользователя).
package streak

// CalculateReward вычисляет бонус в пленках за текущий стрик.
//
// Таблица наград:
//   День 1: 10 пленок
//   День 2: 20 пленок
//   День 3: 30 пленок
//   День 4: 40 пленок
//   День 5: 50 пленок
//   День 6: 60 пленок
//   День 7+: 70 пленок (максимум навсегда)
//
// Параметр currentStreak — текущий стрик ДО увеличения (0 = первый день).
func CalculateReward(currentStreak int) int64 {
	return GetReward(currentStreak)
}

// FormatRewardDescription создаёт описание для транзакции стрик-бонуса.
// Пример: "Streak bonus - Day 8"
func FormatRewardDescription(day int) string {
	return "Streak bonus - Day " + formatDay(day)
}

// formatDay конвертирует число дня в строку.
func formatDay(day int) string {
	// Простой способ — через fmt недоступен тут, делаем вручную
	digits := "0123456789"
	if day < 10 {
		return string(digits[day])
	}
	result := ""
	for day > 0 {
		result = string(digits[day%10]) + result
		day /= 10
	}
	return result
}
