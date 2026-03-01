package telegram

import (
	"context"

	botapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Client инкапсулирует минимум операций Telegram API, которые используются проектом.
type Client interface {
	SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (messageID int, err error)
	EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error
	AnswerCallback(callbackID string) error
	GetChatMember(chatID int64, userID int64) (member models.ChatMember, err error)
}

// Adapter реализует Client через github.com/go-telegram/bot.
type Adapter struct {
	bot *botapi.Bot
}

func NewAdapter(bot *botapi.Bot) *Adapter {
	return &Adapter{bot: bot}
}

func (a *Adapter) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	msg, err := a.bot.SendMessage(context.Background(), &botapi.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: markup,
	})
	if err != nil {
		return 0, err
	}
	if msg == nil {
		return 0, nil
	}
	return msg.ID, nil
}

func (a *Adapter) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	_, err := a.bot.EditMessageText(context.Background(), &botapi.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ReplyMarkup: markup,
	})
	return err
}

func (a *Adapter) AnswerCallback(callbackID string) error {
	if callbackID == "" {
		return nil
	}
	_, err := a.bot.AnswerCallbackQuery(context.Background(), &botapi.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
	})
	return err
}

func (a *Adapter) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	cm, err := a.bot.GetChatMember(context.Background(), &botapi.GetChatMemberParams{
		ChatID: chatID,
		UserID: userID,
	})
	if err != nil {
		return models.ChatMember{}, err
	}
	if cm == nil {
		return models.ChatMember{}, nil
	}
	return *cm, nil
}
