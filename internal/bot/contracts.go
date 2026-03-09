package bot

import (
	"context"
	"time"

	models "github.com/mymmrac/telego"
)

// AdminHandler описывает обработку административных сообщений и callback-й.
type AdminHandler interface {
	HandleAdminCallback(ctx context.Context, cb *models.CallbackQuery) bool
	HandleAdminMessage(ctx context.Context, chatID int64, userID int64, messageID int, text string) bool
	HandleRiddleMessage(ctx context.Context, message *models.Message) bool
}

// MembersHandler описывает обработку callback-ов пользовательского списка участников.
type MembersHandler interface {
	HandleMembersCallback(ctx context.Context, q *models.CallbackQuery) bool
}

// EconomyHandler описывает обработку переводов плёнок вне обычного command-router.
type EconomyHandler interface {
	HandleEconomyCallback(ctx context.Context, q *models.CallbackQuery) bool
	HandleEconomyMessage(ctx context.Context, message *models.Message) bool
}

// ChatAccessFilter описывает проверку доступа апдейтов по чату.
type ChatAccessFilter interface {
	CheckAccess(ctx context.Context, message *models.Message) bool
}

// MemberService описывает операции с участниками чата, которые нужны боту.
type MemberService interface {
	EnsureActiveMemberSeen(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error
	UpsertActiveMember(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error
	MarkMemberLeft(ctx context.Context, userID int64, leftAt, purgeAt time.Time) error
	HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string, isBot bool) error
	CountMembersByStatus(ctx context.Context) (active int, left int, err error)
	CountPendingPurge(ctx context.Context, now time.Time) (pending int, err error)
}

// EconomyService описывает функции экономики, используемые ботом.
type EconomyService interface {
	CreateBalance(ctx context.Context, userID int64) error
}

// StreakService описывает операции streak, используемые ботом.
type StreakService interface {
	CountMessage(ctx context.Context, userID int64, messageID int64, text string) error
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
