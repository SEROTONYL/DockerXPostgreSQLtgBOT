package telegram

import botapi "github.com/mymmrac/telego"

// NewAdapter оставлен для обратной совместимости; используйте NewBotClient.
func NewAdapter(bot *botapi.Bot) Client {
	return NewBotClient(bot)
}
