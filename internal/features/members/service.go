package members

import (
	"context"
	"fmt"
	"strings"
	"time"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

const leftGracePeriod = 5 * 24 * time.Hour

type memberRepository interface {
	UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error
	MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error
	IsActiveMember(ctx context.Context, userID int64) (bool, error)
	PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error)
	GetByUserID(ctx context.Context, userID int64) (*Member, error)
	GetByUsername(ctx context.Context, username string) (*Member, error)
	FindByNickname(ctx context.Context, nickname string) (*Member, error)
	EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error
	EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error
	TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error
	ListActiveUserIDs(ctx context.Context) ([]int64, error)
	ListRefreshCandidateUserIDs(ctx context.Context) ([]int64, error)
	UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error
	CountMembersByStatus(ctx context.Context) (active int, left int, err error)
	CountPendingPurge(ctx context.Context, now time.Time) (int, error)
	GetUsersWithRole(ctx context.Context) ([]*Member, error)
	GetUsersWithoutRole(ctx context.Context) ([]*Member, error)
}

type Service struct {
	repo memberRepository
}

func NewService(repo memberRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string, isBot bool) error {
	return s.UpsertActiveMember(ctx, userID, username, userDisplayName(models.User{FirstName: firstName, LastName: lastName}), isBot, time.Now().UTC())
}

func (s *Service) UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error {
	if err := s.repo.UpsertActiveMember(ctx, userID, username, name, isBot, joinedAt); err != nil {
		return fmt.Errorf("failed to upsert active member: %w", err)
	}
	return nil
}

func (s *Service) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	if err := s.repo.MarkMemberLeft(ctx, userID, leftAt, deleteAfter); err != nil {
		return fmt.Errorf("failed to mark member left: %w", err)
	}
	return nil
}

func (s *Service) MarkMemberLeftNow(ctx context.Context, userID int64) error {
	leftAt := time.Now().UTC()
	return s.MarkMemberLeft(ctx, userID, leftAt, leftAt.Add(leftGracePeriod))
}

func (s *Service) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	if err := s.repo.EnsureMemberSeen(ctx, userID, username, name, isBot, seenAt.UTC()); err != nil {
		return fmt.Errorf("failed to ensure member seen: %w", err)
	}
	return nil
}

func (s *Service) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	if err := s.repo.EnsureActiveMemberSeen(ctx, userID, username, name, isBot, seenAt.UTC()); err != nil {
		return fmt.Errorf("failed to ensure active member seen: %w", err)
	}
	return nil
}

func (s *Service) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	if err := s.repo.TouchLastSeen(ctx, userID, seenAt.UTC()); err != nil {
		return fmt.Errorf("failed to touch last seen: %w", err)
	}
	return nil
}

func (s *Service) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return s.repo.CountMembersByStatus(ctx)
}

func (s *Service) GetUsersWithRole(ctx context.Context) ([]*Member, error) {
	return s.repo.GetUsersWithRole(ctx)
}

func (s *Service) GetUsersWithoutRole(ctx context.Context) ([]*Member, error) {
	return s.repo.GetUsersWithoutRole(ctx)
}

func (s *Service) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return s.repo.CountPendingPurge(ctx, now)
}

func (s *Service) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	return s.repo.ListActiveUserIDs(ctx)
}

func (s *Service) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	if err := s.repo.UpdateMemberTag(ctx, userID, tag, updatedAt.UTC()); err != nil {
		return fmt.Errorf("failed to update member tag: %w", err)
	}
	return nil
}

func (s *Service) ScanAndUpdateMemberTags(ctx context.Context, tgOps *telegram.Ops, memberSourceChatID int64, now time.Time) (int, error) {
	if tgOps == nil || memberSourceChatID == 0 {
		return 0, nil
	}

	activeUserIDs, err := s.ListActiveUserIDs(ctx)
	if err != nil {
		return 0, err
	}
	refreshCandidateUserIDs, err := s.repo.ListRefreshCandidateUserIDs(ctx)
	if err != nil {
		return 0, err
	}

	_ = activeUserIDs

	updated := 0
	for _, userID := range refreshCandidateUserIDs {
		select {
		case <-ctx.Done():
			return updated, ctx.Err()
		default:
		}

		member, err := tgOps.GetChatMember(ctx, memberSourceChatID, userID)
		if err != nil {
			if ctx.Err() != nil {
				return updated, ctx.Err()
			}
			log.WithError(err).WithField("user_id", userID).Warn("ScanMemberTags: getChatMember failed")
			continue
		}
		if member == nil {
			log.WithField("user_id", userID).Warn("ScanMemberTags: empty chat member payload")
			continue
		}

		if !isMemberLikeStatus(member.MemberStatus()) {
			continue
		}

		user := member.MemberUser()
		if err := s.repo.UpsertActiveMember(ctx, userID, user.Username, userDisplayName(user), user.IsBot, now); err != nil {
			if ctx.Err() != nil {
				return updated, ctx.Err()
			}
			log.WithError(err).WithField("user_id", userID).Warn("ScanMemberTags: upsert active identity failed")
			continue
		}

		tag := tgOps.ExtractMemberTag(member)
		if err := s.UpdateMemberTag(ctx, userID, tag, now); err != nil {
			if ctx.Err() != nil {
				return updated, ctx.Err()
			}
			log.WithError(err).WithField("user_id", userID).Warn("ScanMemberTags: update tag failed")
			continue
		}
		updated++
	}

	return updated, nil
}

func (s *Service) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	return s.repo.IsActiveMember(ctx, userID)
}

func (s *Service) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	deleted, err := s.repo.PurgeExpiredLeftMembers(ctx, now, limit)
	if err != nil {
		return 0, fmt.Errorf("failed to purge members: %w", err)
	}
	return deleted, nil
}

func (s *Service) IsMember(ctx context.Context, userID int64) (bool, error) {
	return s.IsActiveMember(ctx, userID)
}

func (s *Service) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *Service) GetRoleAndTag(ctx context.Context, userID int64) (role *string, tag *string, err error) {
	member, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get member role/tag: %w", err)
	}
	if member == nil {
		return nil, nil, nil
	}
	return member.Role, member.Tag, nil
}

func (s *Service) GetByUsername(ctx context.Context, username string) (*Member, error) {
	return s.repo.GetByUsername(ctx, username)
}

func (s *Service) FindByNickname(ctx context.Context, nickname string) (*Member, error) {
	return s.repo.FindByNickname(ctx, nickname)
}

func (s *Service) EnsureMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	if err := s.UpsertActiveMember(ctx, userID, username, userDisplayName(models.User{FirstName: firstName, LastName: lastName}), false, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to ensure member (user_id=%d): %w", userID, err)
	}

	log.WithFields(log.Fields{
		"user_id":  userID,
		"username": username,
	}).Debug("member upserted as active (EnsureMember)")

	return nil
}

func isMemberLikeStatus(status string) bool {
	switch status {
	case "creator", "administrator", "member", "restricted":
		return true
	default:
		return false
	}
}

func userDisplayName(user models.User) string {
	name := strings.TrimSpace(user.FirstName)
	if ln := strings.TrimSpace(user.LastName); ln != "" {
		if name != "" {
			name += " "
		}
		name += ln
	}
	return name
}
