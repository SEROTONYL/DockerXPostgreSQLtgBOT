package telegram

import botapi "github.com/mymmrac/telego"

func NewRawBot(token string) (*botapi.Bot, error) {
	return botapi.NewBot(token)
}
