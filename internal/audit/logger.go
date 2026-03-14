package audit

import (
	"context"
	"fmt"
	"strings"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type MemberLookup interface {
	GetByUserID(ctx context.Context, userID int64) (*members.Member, error)
}

type BalanceChange struct {
	TargetLabel string
	Delta       int64
	NewBalance  int64
}

type Logger struct {
	ops    *telegram.Ops
	chatID int64
}

func NewLogger(ops *telegram.Ops, chatID int64) *Logger {
	return &Logger{ops: ops, chatID: chatID}
}

func (l *Logger) ResolveMemberLabel(ctx context.Context, lookup MemberLookup, userID int64) string {
	if lookup != nil {
		if member, err := lookup.GetByUserID(ctx, userID); err == nil && member != nil {
			return MemberLabel(member)
		}
	}
	return fmt.Sprintf("id:%d", userID)
}

func MemberLabel(member *members.Member) string {
	if member == nil {
		return "id:0"
	}
	if username := strings.TrimSpace(strings.TrimPrefix(member.Username, "@")); username != "" {
		return "@" + username
	}
	if member.Tag != nil {
		if tag := strings.TrimSpace(*member.Tag); tag != "" {
			return tag
		}
	}
	if label := strings.TrimSpace(members.DisplayLabel(member)); label != "" {
		return label
	}
	return fmt.Sprintf("id:%d", member.UserID)
}

func TelegramUserLabel(user *models.User) string {
	if user == nil {
		return "id:0"
	}
	if username := strings.TrimSpace(strings.TrimPrefix(user.Username, "@")); username != "" {
		return "@" + username
	}
	name := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(user.FirstName),
		strings.TrimSpace(user.LastName),
	}, " "))
	if name != "" {
		return name
	}
	return fmt.Sprintf("id:%d", user.ID)
}

func (l *Logger) LogLogin(ctx context.Context, actor string) {
	l.send(ctx, fmt.Sprintf("🔐 Login: %s", actor))
}

func (l *Logger) LogBalanceAdjust(ctx context.Context, actor string, delta int64, changes []BalanceChange) {
	if len(changes) == 0 {
		return
	}
	event := "give"
	if delta < 0 {
		event = "deduct"
	}
	lines := []string{fmt.Sprintf("💸 %s (%+d) by %s", event, delta, actor)}
	for _, change := range changes {
		lines = append(lines, fmt.Sprintf("%s %+d -> %s", change.TargetLabel, change.Delta, common.FormatBalance(change.NewBalance)))
	}
	l.send(ctx, strings.Join(lines, "\n"))
}

func (l *Logger) LogTransfer(ctx context.Context, from, to string, amount int64) {
	l.send(ctx, fmt.Sprintf("💸 transfer: %s -> %s (%d)", from, to, amount))
}

func (l *Logger) LogRoleAssign(ctx context.Context, actor, target, role string) {
	l.send(ctx, fmt.Sprintf("👤 set_role: %s -> %s = %s", actor, target, strings.TrimSpace(role)))
}

func (l *Logger) LogRoleChange(ctx context.Context, actor, target, oldRole, newRole string) {
	l.send(ctx, fmt.Sprintf("🔁 change_role: %s -> %s = %s -> %s", actor, target, normalizeRole(oldRole), normalizeRole(newRole)))
}

func (l *Logger) LogRiddleCreated(ctx context.Context, actor string, reward int64, winners int) {
	l.send(ctx, fmt.Sprintf("🧩 riddle: создана (%d, winners=%d) by %s", reward, winners, actor))
}

func (l *Logger) LogRiddleEnded(ctx context.Context, winners int, reward int64, manual bool) {
	prefix := "✅"
	state := "riddle_end:"
	if manual {
		prefix = "⏹"
		state = "riddle_end: stopped"
	}
	l.send(ctx, fmt.Sprintf("%s %s winners=%d reward=%d", prefix, state, winners, reward))
}

func (l *Logger) send(ctx context.Context, text string) {
	if l == nil || l.ops == nil || l.chatID == 0 || strings.TrimSpace(text) == "" {
		return
	}
	if _, err := l.ops.Send(ctx, l.chatID, text, nil); err != nil {
		log.WithError(err).WithField("admin_chat_id", l.chatID).Warn("audit log send failed")
	}
}

func normalizeRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "—"
	}
	return role
}
