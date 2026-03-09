package streak

import "time"

const (
	dailyMessageTarget = 4
	maxRewardDay       = 7
	maxValidPerMinute  = 2
	duplicateWindow    = 3 * time.Second
)

type Streak struct {
	ID                   int64      `db:"id"`
	UserID               int64      `db:"user_id"`
	CurrentStreak        int        `db:"current_streak"`
	LongestStreak        int        `db:"longest_streak"`
	MessagesToday        int        `db:"messages_today"`
	QuotaCompletedToday  bool       `db:"quota_completed_today"`
	LastQuotaCompletion  *time.Time `db:"last_quota_completion"`
	ProgressDate         *time.Time `db:"progress_date"`
	LastRewardedDay      *time.Time `db:"last_rewarded_day"`
	LastMessageAt        *time.Time `db:"last_message_at"`
	TotalQuotasCompleted int        `db:"total_quotas_completed"`
	ReminderSentToday    bool       `db:"reminder_sent_today"`
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
}

type TopEntry struct {
	UserID        int64
	CurrentStreak int
}

func GetReward(currentStreak int) int64 {
	day := currentStreak + 1
	if day > maxRewardDay {
		day = maxRewardDay
	}
	return int64(day * 10)
}
