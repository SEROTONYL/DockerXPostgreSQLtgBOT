package bot

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
)

// AdminHandler описывает обработку административных сообщений и callback-й.
type AdminHandler interface {
	HandleAdminCallback(ctx context.Context, cb *models.CallbackQuery) bool
	HandleAdminMessage(ctx context.Context, chatID int64, userID int64, text string) bool
}

// MemberService описывает операции с участниками чата, которые нужны боту.
type MemberService interface {
	IsMember(ctx context.Context, userID int64) (bool, error)
	EnsureMember(ctx context.Context, userID int64, username, firstName, lastName string) error
	EnsureActiveMemberSeen(ctx context.Context, userID int64, username, fullName string, now time.Time) error
	UpsertActiveMember(ctx context.Context, userID int64, username, fullName string, now time.Time) error
	MarkMemberLeft(ctx context.Context, userID int64, leftAt, purgeAt time.Time) error
	HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string) error
	CountMembersByStatus(ctx context.Context) (active int, left int, err error)
	CountPendingPurge(ctx context.Context, now time.Time) (pending int, err error)
}

// EconomyService описывает функции экономики, используемые ботом.
type EconomyService interface {
	CreateBalance(ctx context.Context, userID int64) error
}

// StreakService описывает операции streak, используемые ботом.
type StreakService interface {
	CountMessage(ctx context.Context, userID int64, text string)
	CreateStreak(ctx context.Context, userID int64) error
}

// KarmaService описывает операции karma, используемые ботом.
type KarmaService interface {
	CreateKarma(ctx context.Context, userID int64) error
}

// KarmaHandler описывает обработку thank-you событий.
type KarmaHandler interface {
	HandleThankYou(ctx context.Context, chatID int64, fromUserID int64, toUserID int64)
}

// KarmaThankYouClassifier определяет, является ли сообщение благодарностью.
type KarmaThankYouClassifier interface {
	IsThankYou(text string) bool
}
