package admin

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/audit"
)

func (h *Handler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

func (h *Handler) auditActorLabel(ctx context.Context, userID int64) string {
	if h == nil || h.audit == nil || h.service == nil {
		return "id:0"
	}
	return h.audit.ResolveMemberLabel(ctx, h.service.memberRepo, userID)
}
