package bot

import (
	"context"
	"time"

	models "github.com/mymmrac/telego"
)

type AdminHandler interface {
	HandleAdminCallback(ctx context.Context, cb *models.CallbackQuery) bool
	HandleAdminMessage(ctx context.Context, chatID int64, userID int64, messageID int, text string) bool
	HandleRiddleMessage(ctx context.Context, message *models.Message) bool
}

type MembersHandler interface {
	HandleMembersCallback(ctx context.Context, q *models.CallbackQuery) bool
}

type EconomyHandler interface {
	HandleEconomyCallback(ctx context.Context, q *models.CallbackQuery) bool
	HandleEconomyMessage(ctx context.Context, message *models.Message) bool
}

type ChatAccessFilter interface {
	CheckAccess(ctx context.Context, message *models.Message) bool
}

type MemberService interface {
	EnsureActiveMemberSeen(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error
	UpsertActiveMember(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error
	MarkMemberLeft(ctx context.Context, userID int64, leftAt, purgeAt time.Time) error
	GetRoleAndTag(ctx context.Context, userID int64) (role *string, tag *string, err error)
	HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string, isBot bool) error
	CountMembersByStatus(ctx context.Context) (active int, left int, err error)
	CountPendingPurge(ctx context.Context, now time.Time) (pending int, err error)
}

type EconomyService interface {
	CreateBalance(ctx context.Context, userID int64) error
}

type StreakService interface {
	CountMessage(ctx context.Context, userID int64, messageID int64, text string) error
	CreateStreak(ctx context.Context, userID int64) error
}

type KarmaService interface {
	CreateKarma(ctx context.Context, userID int64) error
}

type KarmaHandler interface {
	HandleThankYou(ctx context.Context, chatID int64, fromUserID int64, toUserID int64)
}

type KarmaThankYouClassifier interface {
	IsThankYou(text string) bool
}
