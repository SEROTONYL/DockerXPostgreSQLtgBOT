// Package members — service.go содержит бизнес-логику управления участниками.
// Сервис координирует регистрацию новых участников, проверку членства
// и обновление информации.
package members

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
)

// Service управляет участниками чата.
// Связывает обработчики Telegram-событий с репозиторием БД.
type Service struct {
	repo *Repository // Репозиторий для работы с таблицей members
}

// NewService создаёт новый сервис участников.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// HandleNewMember обрабатывает вступление нового пользователя в чат.
// Если пользователь уже есть в базе (перезашёл) — обновляет его данные.
// Если пользователь новый — создаёт запись.
func (s *Service) HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("ошибка чтения участника из БД (user_id=%d): %w", userID, err)
	}
	if existing != nil {
		log.WithField("user_id", userID).Info("Участник перезашёл в чат, обновляем данные")
		return s.repo.UpdateInfo(ctx, userID, UpdateInfo{
			Username:  username,
			FirstName: firstName,
			LastName:  lastName,
		})
	}

	// Если ошибка НЕ "не найдено" — это реально проблема БД, не надо делать вид, что всё норм.
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ошибка чтения участника (user_id=%d): %w", userID, err)
	}

	// Создаём нового участника
	member := &Member{
		UserID:    userID,
		Username:  username,
		FirstName: firstName,
		LastName:  lastName,
		IsAdmin:   false,
		IsBanned:  false,
	}

	if err := s.repo.Create(ctx, member); err != nil {
		return fmt.Errorf("ошибка регистрации нового участника: %w", err)
	}

	log.WithFields(log.Fields{
		"user_id":  userID,
		"username": username,
	}).Info("Новый участник зарегистрирован")

	return nil
}

// IsMember проверяет, является ли пользователь участником чата.
// Используется для валидации доступа к DM.
func (s *Service) IsMember(ctx context.Context, userID int64) (bool, error) {
	return s.repo.Exists(ctx, userID)
}

// GetByUserID возвращает участника по его Telegram user ID.
func (s *Service) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	return s.repo.GetByUserID(ctx, userID)
}

// GetByUsername возвращает участника по @username (без @).
func (s *Service) GetByUsername(ctx context.Context, username string) (*Member, error) {
	return s.repo.GetByUsername(ctx, username)
}

// EnsureMember гарантирует, что пользователь есть в базе.
// Если нет — создаёт запись. Используется при первом сообщении в чате и при DM-backfill.
func (s *Service) EnsureMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	// Можно было бы сделать просто Create() (у тебя ON CONFLICT DO NOTHING),
	// но оставим Exists() как "быстрый выход" и понятные логи.
	exists, err := s.repo.Exists(ctx, userID)
	if err != nil {
		return fmt.Errorf("ошибка проверки существования участника (user_id=%d): %w", userID, err)
	}
	if exists {
		return nil
	}

	member := &Member{
		UserID:    userID,
		Username:  username,
		FirstName: firstName,
		LastName:  lastName,
		IsAdmin:   false,
		IsBanned:  false,
	}

	if err := s.repo.Create(ctx, member); err != nil {
		return fmt.Errorf("ошибка ensure участника (user_id=%d): %w", userID, err)
	}

	log.WithFields(log.Fields{
		"user_id":  userID,
		"username": username,
	}).Info("Участник бэкфилен в БД (EnsureMember)")

	return nil
}