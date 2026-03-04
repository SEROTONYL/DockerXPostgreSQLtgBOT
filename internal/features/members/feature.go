package members

import "serotonyl.ru/telegram-bot/internal/commands"

type Feature struct{}

func NewFeature() *Feature                           { return &Feature{} }
func (f *Feature) Name() string                      { return "members" }
func (f *Feature) RegisterCommands(*commands.Router) {}
