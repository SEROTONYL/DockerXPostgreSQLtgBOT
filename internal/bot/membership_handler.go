package bot

import (
	"context"
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
	if cmu.Chat.ID != b.cfg.MainGroupID {
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

	switch classifyMemberStatus(newStatus) {
	case membershipActionActive:
		if err := b.memberService.UpsertActiveMember(ctx, user.ID, user.Username, name, now); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("UpsertActiveMember failed")
			return true
		}
		if classifyMemberStatus(oldStatus) != membershipActionActive {
			b.handleNewMembers(ctx, []models.User{*user})
		}
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "active"}).Info("membership transition handled")
	case membershipActionLeft:
		if err := b.memberService.MarkMemberLeft(ctx, user.ID, now, now.Add(5*24*time.Hour)); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("MarkMemberLeft failed")
			return true
		}
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "left"}).Info("membership transition handled")
	default:
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "ignore"}).Debug("membership transition ignored")
	}

	return true
}

// handleNewMembers обрабатывает вступление новых участников.
func (b *Bot) handleNewMembers(ctx context.Context, newMembers []models.User) {
	for _, user := range newMembers {
		if err := b.memberService.HandleNewMember(ctx, user.ID, user.Username, user.FirstName, user.LastName); err != nil {
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

		log.WithField("user", user.Username).Info("Новый участник обработан")
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
