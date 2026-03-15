package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"
)

func (b *Bot) handleMembershipUpdate(ctx context.Context, uc UpdateContext) bool {
	cmu := uc.ChatMember
	if cmu == nil {
		return false
	}
	if cmu.Chat.ID != b.cfg.MemberSourceChatID {
		return true
	}

	oldStatus := cmu.OldChatMember.MemberStatus()
	newStatus := cmu.NewChatMember.MemberStatus()
	user, ok := chatMemberUser(cmu.NewChatMember)
	if !ok {
		log.WithFields(log.Fields{"old_status": oldStatus, "new_status": newStatus, "chat_id": cmu.Chat.ID}).Warn("membership update without user payload")
		return true
	}

	name := buildDisplayName(user.FirstName, user.LastName)
	now := uc.Now
	oldAction := classifyMemberStatus(oldStatus)
	newAction := classifyMemberStatus(newStatus)

	switch newAction {
	case membershipActionActive:
		if err := b.memberService.UpsertActiveMember(ctx, user.ID, user.Username, name, user.IsBot, now); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("UpsertActiveMember failed")
			return true
		}
		if oldAction != membershipActionActive {
			b.handleNewMembers(ctx, []models.User{*user})
		}
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "active"}).Info("membership transition handled")
	case membershipActionLeft:
		if err := b.memberService.MarkMemberLeft(ctx, user.ID, now, now.Add(5*24*time.Hour)); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("MarkMemberLeft failed")
			return true
		}
		if oldAction != membershipActionLeft {
			b.notifyLeaveDebug(ctx, user)
		}
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "left"}).Info("membership transition handled")
	default:
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "ignore"}).Debug("membership transition ignored")
	}

	return true
}

func (b *Bot) notifyLeaveDebug(ctx context.Context, user *models.User) {
	if b == nil || b.cfg == nil || b.ops == nil || b.memberService == nil || user == nil {
		return
	}
	if b.cfg.LeaveDebug <= 0 {
		return
	}

	role, tag, err := b.memberService.GetRoleAndTag(ctx, user.ID)
	if err != nil {
		log.WithError(err).WithField("leaving_user_id", user.ID).Warn("leave debug member lookup failed")
	}

	text := fmt.Sprintf("#Выход — %s (%s)", leaveDebugLabel(role, tag), leaveDebugIdentity(user))
	if _, err := b.ops.Send(ctx, b.cfg.LeaveDebug, text, nil); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"leaving_user_id":     user.ID,
			"leave_debug_user_id": b.cfg.LeaveDebug,
		}).Warn("leave debug notification failed")
	}
}

func leaveDebugLabel(role *string, tag *string) string {
	if role != nil {
		if trimmed := strings.TrimSpace(*role); trimmed != "" {
			return trimmed
		}
	}
	if tag != nil {
		return *tag
	}
	return "Без роли"
}

func leaveDebugIdentity(user *models.User) string {
	if user == nil {
		return ""
	}
	if username := strings.TrimSpace(user.Username); username != "" {
		return "@" + username
	}
	return fmt.Sprintf("%d", user.ID)
}

func (b *Bot) handleNewMembers(ctx context.Context, newMembers []models.User) {
	for _, user := range newMembers {
		if err := b.memberService.HandleNewMember(ctx, user.ID, user.Username, user.FirstName, user.LastName, user.IsBot); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("HandleNewMember failed")
		}
		if err := b.economyService.CreateBalance(ctx, user.ID); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("CreateBalance failed")
		}
		if err := b.streakService.CreateStreak(ctx, user.ID); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("CreateStreak failed")
		}
		if err := b.karmaService.CreateKarma(ctx, user.ID); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("CreateKarma failed")
		}

		log.WithField("user", user.Username).Info("new member handled")
	}
}

type membershipAction string

const (
	membershipActionIgnore membershipAction = "ignore"
	membershipActionActive membershipAction = "active"
	membershipActionLeft   membershipAction = "left"
)

func classifyMemberStatus(status string) membershipAction {
	switch status {
	case "creator", "administrator", "member", "restricted":
		return membershipActionActive
	case "left", "kicked":
		return membershipActionLeft
	default:
		return membershipActionIgnore
	}
}

func extractChatMemberUpdate(update models.Update) *models.ChatMemberUpdated {
	if update.ChatMember != nil {
		return update.ChatMember
	}
	if update.MyChatMember != nil {
		return update.MyChatMember
	}
	return nil
}

func chatMemberUser(member models.ChatMember) (*models.User, bool) {
	if member == nil {
		return nil, false
	}
	u := member.MemberUser()
	if u.ID == 0 {
		return nil, false
	}
	return &u, true
}

func buildDisplayName(firstName, lastName string) string {
	name := strings.TrimSpace(firstName)
	if ln := strings.TrimSpace(lastName); ln != "" {
		if name != "" {
			name += " "
		}
		name += ln
	}
	return name
}
