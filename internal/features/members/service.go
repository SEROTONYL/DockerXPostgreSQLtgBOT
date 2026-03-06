// Package members — service.go содержит бизнес-логику управления участниками.
// Сервис координирует регистрацию новых участников, проверку членства
// и обновление информации.
package members

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

const leftGracePeriod = 5 * 24 * time.Hour

// Service управляет участниками чата.
// Связывает обработчики Telegram-событий с репозиторием БД.
type memberRepository interface {
	UpsertActiveMember(ctx context.Context, userID int64, username, name string, joinedAt time.Time) error
	MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error
	IsActiveMember(ctx context.Context, userID int64) (bool, error)
	PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error)
	GetByUserID(ctx context.Context, userID int64) (*Member, error)
	GetByUsername(ctx context.Context, username string) (*Member, error)
	EnsureMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error
	EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error
	TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error
	ListActiveUserIDs(ctx context.Context) ([]int64, error)
	UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error
	CountMembersByStatus(ctx context.Context) (active int, left int, err error)
	CountPendingPurge(ctx context.Context, now time.Time) (int, error)
}

// Service управляет участниками чата.
// Связывает обработчики Telegram-событий с репозиторием БД.
type Service struct {
	repo memberRepository // Репозиторий для работы с таблицей members
}

// NewService создаёт новый сервис участников.
func NewService(repo memberRepository) *Service {
	return &Service{repo: repo}
}

// HandleNewMember обрабатывает вступление нового пользователя в чат.
func (s *Service) HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	return s.UpsertActiveMember(ctx, userID, username, firstName, time.Now().UTC())
}

// UpsertActiveMember вставляет/обновляет участника и помечает его как active.
func (s *Service) UpsertActiveMember(ctx context.Context, userID int64, username, name string, joinedAt time.Time) error {
	if err := s.repo.UpsertActiveMember(ctx, userID, username, name, joinedAt); err != nil {
		return fmt.Errorf("ошибка upsert участника: %w", err)
	}
	return nil
}

// MarkMemberLeft помечает участника как left и выставляет окно grace period.
func (s *Service) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	if err := s.repo.MarkMemberLeft(ctx, userID, leftAt, deleteAfter); err != nil {
		return fmt.Errorf("ошибка пометки участника left: %w", err)
	}
	return nil
}

// MarkMemberLeftNow помечает участника как left с grace period 5 дней.
func (s *Service) MarkMemberLeftNow(ctx context.Context, userID int64) error {
	leftAt := time.Now().UTC()
	return s.MarkMemberLeft(ctx, userID, leftAt, leftAt.Add(leftGracePeriod))
}

// EnsureMemberSeen обновляет известные данные/last_seen только для уже существующего участника.
// Строгий режим: если записи нет (например, DM "из воздуха"), создаём ничего и возвращаем nil.
func (s *Service) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
	if err := s.repo.EnsureMemberSeen(ctx, userID, username, name, seenAt.UTC()); err != nil {
		return fmt.Errorf("ошибка ensure member seen: %w", err)
	}
	return nil
}

// EnsureActiveMemberSeen обновляет/создаёт участника как active (для апдейтов из основной группы).
func (s *Service) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
	if err := s.repo.EnsureActiveMemberSeen(ctx, userID, username, name, seenAt.UTC()); err != nil {
		return fmt.Errorf("ошибка ensure active member seen: %w", err)
	}
	return nil
}

// TouchLastSeen обновляет только last_seen_at с SQL-троттлингом и безопасен при 0 affected rows.
func (s *Service) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	if err := s.repo.TouchLastSeen(ctx, userID, seenAt.UTC()); err != nil {
		return fmt.Errorf("ошибка touch last seen: %w", err)
	}
	return nil
}

func (s *Service) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return s.repo.CountMembersByStatus(ctx)
}

func (s *Service) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return s.repo.CountPendingPurge(ctx, now)
}

func (s *Service) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	return s.repo.ListActiveUserIDs(ctx)
}

func (s *Service) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	if err := s.repo.UpdateMemberTag(ctx, userID, tag, updatedAt.UTC()); err != nil {
		return fmt.Errorf("ошибка обновления tag участника: %w", err)
	}
	return nil
}

// ScanAndUpdateMemberTags вручную обновляет metadata/tag только для уже известных active-участников из БД,
// но не предназначен для обнаружения новых участников Telegram.
func (s *Service) ScanAndUpdateMemberTags(ctx context.Context, tgOps *telegram.Ops, mainGroupID int64, now time.Time) (int, error) {
	if tgOps == nil || mainGroupID == 0 {
		return 0, nil
	}

	userIDs, err := s.ListActiveUserIDs(ctx)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, userID := range userIDs {
		select {
		case <-ctx.Done():
			return updated, ctx.Err()
		default:
		}

		member, err := tgOps.GetChatMember(ctx, mainGroupID, userID)
		if err != nil {
			if ctx.Err() != nil {
				return updated, ctx.Err()
			}
			log.WithError(err).WithField("user_id", userID).Warn("ScanMemberTags: getChatMember failed")
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

// IsActiveMember проверяет активность участника.
func (s *Service) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	return s.repo.IsActiveMember(ctx, userID)
}

// PurgeExpiredLeftMembers жёстко удаляет вышедших участников с истекшим delete_after.
func (s *Service) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	deleted, err := s.repo.PurgeExpiredLeftMembers(ctx, now, limit)
	if err != nil {
		return 0, fmt.Errorf("ошибка purge участников: %w", err)
	}
	return deleted, nil
}

// IsMember проверяет, является ли пользователь участником чата.
// Используется для валидации доступа к DM.
func (s *Service) IsMember(ctx context.Context, userID int64) (bool, error) {
	return s.IsActiveMember(ctx, userID)
}

// GetByUserID возвращает участника по его Telegram user ID.
func (s *Service) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	return s.repo.GetByUserID(ctx, userID)
}

// GetByUsername возвращает участника по @username (без @).
func (s *Service) GetByUsername(ctx context.Context, username string) (*Member, error) {
	return s.repo.GetByUsername(ctx, username)
}

// EnsureMember гарантирует, что пользователь есть в базе и активен.
func (s *Service) EnsureMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	if err := s.UpsertActiveMember(ctx, userID, username, firstName, time.Now().UTC()); err != nil {
		return fmt.Errorf("ошибка ensure участника (user_id=%d): %w", userID, err)
	}

	log.WithFields(log.Fields{
		"user_id":  userID,
		"username": username,
	}).Debug("Участник upsert как active (EnsureMember)")

	return nil
}
