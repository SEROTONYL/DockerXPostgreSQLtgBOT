// Package streak управляет системой ежедневных стриков (серий).
// models.go описывает структуру данных стрика.
package streak

import "time"

// Streak представляет запись стрика пользователя.
// Стрик увеличивается каждый день, когда пользователь пишет
// достаточно сообщений (50 по умолчанию) в чате.
type Streak struct {
	ID                   int64      `db:"id"`
	UserID               int64      `db:"user_id"`
	CurrentStreak        int        `db:"current_streak"`         // Текущая серия (дней подряд)
	LongestStreak        int        `db:"longest_streak"`         // Личный рекорд
	MessagesToday        int        `db:"messages_today"`         // Сообщений сегодня (3+ слов)
	QuotaCompletedToday  bool       `db:"quota_completed_today"`  // Норма выполнена сегодня?
	LastQuotaCompletion  *time.Time `db:"last_quota_completion"`  // Дата последнего выполнения нормы
	LastMessageAt        *time.Time `db:"last_message_at"`        // Время последнего сообщения
	TotalQuotasCompleted int        `db:"total_quotas_completed"` // Всего раз выполнена норма
	ReminderSentToday    bool       `db:"reminder_sent_today"`    // Напоминание отправлено?
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
}

// StreakRewards — бонусы за стрики по дням.
// Индекс массива = текущий стрик (0 = первый день).
// С 7-го дня и далее — 70 пленок.
var StreakRewards = []int64{10, 20, 30, 40, 50, 60, 70}

// GetReward возвращает бонус за текущий стрик.
// День 1 → 10, День 2 → 20, ..., День 7+ → 70
func GetReward(currentStreak int) int64 {
	if currentStreak < len(StreakRewards) {
		return StreakRewards[currentStreak]
	}
	// Начиная с 7-го дня — всегда 70
	return 70
}
