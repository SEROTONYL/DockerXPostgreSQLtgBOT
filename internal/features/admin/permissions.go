package admin

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type permissionSet struct {
	cfg *config.Config
}

func newPermissionSet(cfg *config.Config) *permissionSet {
	return &permissionSet{cfg: cfg}
}

func (p *permissionSet) IsAdmin(userID int64, member *members.Member) bool {
	return p.isEnvAdmin(userID) || p.isDBAdmin(member)
}

func (p *permissionSet) IsModerator(userID int64, member *members.Member) bool {
	if p.IsAdmin(userID, member) {
		return true
	}
	return p.isEnvModerator(userID) || p.isDBModerator(member)
}

func (p *permissionSet) CanAccessAdminPanel(userID int64, member *members.Member) bool {
	return p.IsModerator(userID, member)
}

func (p *permissionSet) CanManageRiddles(userID int64, member *members.Member) bool {
	return p.IsModerator(userID, member)
}

func (p *permissionSet) CanManageRoles(userID int64, member *members.Member) bool {
	return p.IsAdmin(userID, member)
}

func (p *permissionSet) CanManageBalance(userID int64, member *members.Member) bool {
	return p.IsAdmin(userID, member)
}

func (p *permissionSet) CanManageCredits(userID int64, member *members.Member) bool {
	return p.IsAdmin(userID, member)
}

func (p *permissionSet) isEnvAdmin(userID int64) bool {
	if p == nil || p.cfg == nil {
		return false
	}
	if _, ok := p.cfg.AdminIDSet[userID]; ok {
		return true
	}
	for _, id := range p.cfg.AdminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (p *permissionSet) isEnvModerator(userID int64) bool {
	if p == nil || p.cfg == nil {
		return false
	}
	if _, ok := p.cfg.ModeratorIDSet[userID]; ok {
		return true
	}
	for _, id := range p.cfg.ModeratorIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (p *permissionSet) isDBAdmin(member *members.Member) bool {
	return member != nil && member.IsAdmin
}

func (p *permissionSet) isDBModerator(member *members.Member) bool {
	// Reserved for future DB-backed moderator roles.
	return false
}

func (s *Service) permissionMember(ctx context.Context, userID int64) *members.Member {
	if s == nil || s.memberRepo == nil {
		return nil
	}
	member, err := s.memberRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil
	}
	return member
}

func (s *Service) IsAdmin(ctx context.Context, userID int64) bool {
	return s.permissions.IsAdmin(userID, s.permissionMember(ctx, userID))
}

func (s *Service) IsModerator(ctx context.Context, userID int64) bool {
	return s.permissions.IsModerator(userID, s.permissionMember(ctx, userID))
}

func (s *Service) CanAccessAdminPanel(ctx context.Context, userID int64) bool {
	return s.permissions.CanAccessAdminPanel(userID, s.permissionMember(ctx, userID))
}

func (s *Service) CanManageRiddles(ctx context.Context, userID int64) bool {
	return s.permissions.CanManageRiddles(userID, s.permissionMember(ctx, userID))
}

func (s *Service) CanManageRoles(ctx context.Context, userID int64) bool {
	return s.permissions.CanManageRoles(userID, s.permissionMember(ctx, userID))
}

func (s *Service) CanManageBalance(ctx context.Context, userID int64) bool {
	return s.permissions.CanManageBalance(userID, s.permissionMember(ctx, userID))
}

func (s *Service) CanManageCredits(ctx context.Context, userID int64) bool {
	return s.permissions.CanManageCredits(userID, s.permissionMember(ctx, userID))
}
