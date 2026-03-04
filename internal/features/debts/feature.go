package debts

import "serotonyl.ru/telegram-bot/internal/commands"

type Feature struct{}

func NewFeature() *Feature                           { return &Feature{} }
func (f *Feature) Name() string                      { return "debts" }
func (f *Feature) RegisterCommands(*commands.Router) {}
