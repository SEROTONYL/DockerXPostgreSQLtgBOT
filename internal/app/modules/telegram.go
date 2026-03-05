package modules

import (
	"context"
	"fmt"

	bot "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type Telegram struct {
	RawBot *bot.Bot
	Client telegram.Client
	Ops    *telegram.Ops
}

func BuildTelegram(ctx context.Context, cfg *config.Config) (*Telegram, error) {
	botAPI, err := telegram.NewRawBot(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания Telegram API: %w", err)
	}

	tgClient := telegram.NewBotClient(botAPI)
	tgOps := telegram.NewOpsWithLogger(tgClient, log.NewEntry(log.StandardLogger()))

	me, err := tgOps.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("ошибка getMe Telegram API: %w", err)
	}
	log.Infof("Авторизован как @%s", me.Username)

	return &Telegram{RawBot: botAPI, Client: tgClient, Ops: tgOps}, nil
}
