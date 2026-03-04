package admin

import (
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/jobs"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Deps contains dependencies required to build the admin module.
type Deps struct {
	Cfg            *config.Config
	Ops            *telegram.Ops
	Service        *Service
	MemberService  *members.Service
	EconomyService *economy.Service
	PurgeMetrics   func() jobs.PurgeMetrics
}

// Module groups runtime handlers and command feature.
type Module struct {
	Handler *Handler
	Feature feature.Feature
}

// NewModule builds the admin handler and feature.
func NewModule(deps Deps) (*Module, error) {
	h := NewHandler(deps.Service, deps.MemberService, deps.EconomyService, deps.Ops)
	f := NewFeature(deps.Cfg, deps.Ops, h, deps.MemberService, deps.PurgeMetrics)
	return &Module{Handler: h, Feature: f}, nil
}

// Build creates only admin feature implementation.
func Build(deps Deps) (feature.Feature, error) {
	m, err := NewModule(deps)
	if err != nil {
		return nil, err
	}
	return m.Feature, nil
}
