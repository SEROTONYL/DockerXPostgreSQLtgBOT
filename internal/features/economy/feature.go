package economy

import (
	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

type Feature struct {
	h   *Handler
	cfg *config.Config
}

func NewFeature(h *Handler, cfg *config.Config) *Feature { return &Feature{h: h, cfg: cfg} }
func (f *Feature) Name() string                          { return "economy" }
func (f *Feature) RegisterCommands(r *commands.Router)   { RegisterCommands(r, f.h, f.cfg) }
