package telegram

import botapi "github.com/go-telegram/bot"

func NewRawBot(token string) (*botapi.Bot, error) {
	return botapi.New(token)
}
