package karma

import (
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type Deps struct {
	Cfg           *config.Config
	Ops           *telegram.Ops
	Service       *Service
	MemberService *members.Service
}

type Module struct {
	Handler *Handler
	Feature feature.Feature
}

func NewModule(deps Deps) (*Module, error) {
	h := NewHandler(deps.Service, deps.MemberService, deps.Ops)
	f := NewFeature(h, deps.Cfg)
	return &Module{Handler: h, Feature: f}, nil
}

func Build(deps Deps) (feature.Feature, error) {
	m, err := NewModule(deps)
	if err != nil {
		return nil, err
	}
	return m.Feature, nil
}
