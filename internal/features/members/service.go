// Package members — service.go содержит бизнес-логику управления участниками.
// Сервис координирует регистрацию новых участников, проверку членства
// и обновление информации.
package members

import (
	"context"
	"fmt"

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
//
// Параметры:
//   - ctx: контекст
//   - userID: Telegram user ID
//   - username: @username (может быть пустым)
//   - firstName: имя
//   - lastName: фамилия
func (s *Service) HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	// Проверяем, есть ли уже в базе
	existing, _ := s.repo.GetByUserID(ctx, userID)
	if existing != nil {
		// Пользователь уже зарегистрирован — обновляем данные
		log.WithField("user_id", userID).Info("Участник перезашёл в чат, обновляем данные")
		return s.repo.UpdateInfo(ctx, userID, UpdateInfo{
			Username:  username,
			FirstName: firstName,
			LastName:  lastName,
		})
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
// Если нет — создаёт запись. Используется при первом сообщении в чате.
func (s *Service) EnsureMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	exists, err := s.repo.Exists(ctx, userID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.HandleNewMember(ctx, userID, username, firstName, lastName)
}
