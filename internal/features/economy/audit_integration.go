package economy

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/audit"
)

func (h *Handler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

func (h *Handler) auditMemberLabel(ctx context.Context, userID int64) string {
	if h == nil || h.audit == nil {
		return "id:0"
	}
	return h.audit.ResolveMemberLabel(ctx, h.memberService, userID)
}
