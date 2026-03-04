package core

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Feature регистрирует базовые команды бота.
type Feature struct {
	ops *telegram.Ops
}

func NewFeature(ops *telegram.Ops) *Feature {
	return &Feature{ops: ops}
}

func (f *Feature) Name() string { return "core" }

func (f *Feature) RegisterCommands(r *commands.Router) {
	r.Register("start", func(ctx context.Context, c commands.Context, args []string) {
		f.sendMessage(ctx, c.ChatID, "Я живой. Команды: /login <пароль> (админ), !плёнки, !карма, !слоты ...")
	})
	r.Register("help", func(ctx context.Context, c commands.Context, args []string) {
		f.sendMessage(ctx, c.ChatID, "Я живой. Команды: /login <пароль> (админ), !плёнки, !карма, !слоты ...")
	})
}

func (f *Feature) sendMessage(ctx context.Context, chatID int64, text string) {
	if f.ops == nil {
		return
	}
	_, _ = f.ops.Send(ctx, chatID, text, nil)
}
