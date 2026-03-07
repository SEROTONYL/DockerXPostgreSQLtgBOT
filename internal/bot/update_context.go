package bot

import (
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/config"
)

type UpdateContext struct {
	Update models.Update
	Now    time.Time

	ChatID   int64
	UserID   int64
	Username string
	FullName string

	IsPrivate   bool
	IsGroup     bool
	IsAdminChat bool

	Message    *models.Message
	Callback   *models.CallbackQuery
	ChatMember *models.ChatMemberUpdated
}

func BuildUpdateContext(update models.Update, now time.Time, cfg *config.Config) UpdateContext {
	uc := UpdateContext{Update: update, Now: now.UTC()}

	if update.Message != nil {
		uc.Message = update.Message
		uc.ChatID = update.Message.Chat.ID
		if update.Message.From != nil {
			uc.UserID = update.Message.From.ID
			uc.Username = update.Message.From.Username
			uc.FullName = buildDisplayName(update.Message.From.FirstName, update.Message.From.LastName)
		}
		uc.IsPrivate = update.Message.Chat.Type == models.ChatTypePrivate
		uc.IsGroup = update.Message.Chat.Type == models.ChatTypeGroup || update.Message.Chat.Type == models.ChatTypeSupergroup
	}

	if update.CallbackQuery != nil {
		uc.Callback = update.CallbackQuery
		if uc.UserID == 0 {
			uc.UserID = update.CallbackQuery.From.ID
			uc.Username = update.CallbackQuery.From.Username
			uc.FullName = buildDisplayName(update.CallbackQuery.From.FirstName, update.CallbackQuery.From.LastName)
		}
		if uc.ChatID == 0 {
			if chat, ok := callbackQueryChat(update.CallbackQuery); ok {
				uc.ChatID = chat.ID
				uc.IsPrivate = chat.Type == models.ChatTypePrivate
				uc.IsGroup = chat.Type == models.ChatTypeGroup || chat.Type == models.ChatTypeSupergroup
			}
		}
	}

	if cmu := extractChatMemberUpdate(update); cmu != nil {
		uc.ChatMember = cmu
		if uc.ChatID == 0 {
			uc.ChatID = cmu.Chat.ID
			uc.IsPrivate = cmu.Chat.Type == models.ChatTypePrivate
			uc.IsGroup = cmu.Chat.Type == models.ChatTypeGroup || cmu.Chat.Type == models.ChatTypeSupergroup
		}
		if uc.UserID == 0 {
			if user, ok := chatMemberUser(cmu.NewChatMember); ok {
				uc.UserID = user.ID
				uc.Username = user.Username
				uc.FullName = buildDisplayName(user.FirstName, user.LastName)
			}
		}
	}

	if cfg != nil && cfg.AdminChatID != 0 && uc.ChatID == cfg.AdminChatID {
		uc.IsAdminChat = true
	}

	return uc
}

func callbackQueryChat(q *models.CallbackQuery) (models.Chat, bool) {
	if q == nil || q.Message == nil {
		return models.Chat{}, false
	}
	if msg := q.Message.Message(); msg != nil {
		return msg.Chat, true
	}
	if msg := q.Message.InaccessibleMessage(); msg != nil {
		return msg.Chat, true
	}
	return models.Chat{}, false
}
