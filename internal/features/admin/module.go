package admin

import (
	"serotonyl.ru/telegram-bot/internal/audit"
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
	RiddleService  *RiddleService
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
	var memberSourceChatID int64
	if deps.Cfg != nil {
		memberSourceChatID = deps.Cfg.MemberSourceChatID
	}
	if deps.Service != nil {
		deps.Service.SetRiddleService(deps.RiddleService)
	}
	if deps.RiddleService != nil {
		deps.RiddleService.SetOps(deps.Ops)
		if deps.Cfg != nil && deps.Service != nil {
			deps.RiddleService.SetAuditLogger(audit.NewLogger(deps.Ops, deps.Cfg.AdminChatID), deps.Service.memberRepo)
		}
	}
	h := NewHandler(deps.Service, deps.MemberService, deps.EconomyService, deps.Ops, memberSourceChatID)
	if deps.Cfg != nil {
		h.SetAuditLogger(audit.NewLogger(deps.Ops, deps.Cfg.AdminChatID))
	}
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
