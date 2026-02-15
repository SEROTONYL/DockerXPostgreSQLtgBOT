// Package admin — service.go содержит логику аутентификации, управления сессиями
// и state-машину для пошаговых админ-действий.
package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/argon2"

	"telegram-bot/internal/config"
	"telegram-bot/internal/features/members"
)

// Service управляет админ-панелью.
type Service struct {
	repo       *Repository
	memberRepo *members.Repository
	cfg        *config.Config
	states     map[int64]*AdminState // Состояния диалогов (in-memory)
	statesMu   sync.RWMutex
}

// NewService создаёт сервис админ-панели.
func NewService(repo *Repository, memberRepo *members.Repository, cfg *config.Config) *Service {
	return &Service{
		repo:       repo,
		memberRepo: memberRepo,
		cfg:        cfg,
		states:     make(map[int64]*AdminState),
	}
}

// VerifyPassword проверяет пароль администратора с использованием Argon2id.
// Включает защиту от brute-force: 3 неудачные попытки = блокировка на 1 час.
func (s *Service) VerifyPassword(ctx context.Context, userID int64, password string) error {
	// Проверяем лимит попыток
	attempts, err := s.repo.GetRecentAttempts(ctx, userID, 1*time.Hour)
	if err != nil {
		return err
	}
	if attempts >= 3 {
		return fmt.Errorf("слишком много попыток, подождите 1 час")
	}

	// Проверяем пароль
	match := verifyArgon2id(password, s.cfg.AdminPasswordHash)

	// Логируем попытку
	s.repo.LogAttempt(ctx, userID, match)

	if !match {
		return fmt.Errorf("неверный пароль")
	}

	// Создаём сессию (24 часа)
	token := generateSecureToken()
	session := &AdminSession{
		UserID:       userID,
		SessionToken: token,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	return s.repo.CreateSession(ctx, session)
}

// HasActiveSession проверяет, есть ли у пользователя активная сессия.
func (s *Service) HasActiveSession(ctx context.Context, userID int64) bool {
	session, err := s.repo.GetActiveSession(ctx, userID)
	return err == nil && session != nil
}

// GetState возвращает текущее состояние диалога.
func (s *Service) GetState(userID int64) *AdminState {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()

	state, ok := s.states[userID]
	if !ok {
		return nil
	}
	// Проверяем истечение
	if time.Now().After(state.ExpiresAt) {
		return nil
	}
	return state
}

// SetState устанавливает состояние диалога с 5-минутным таймаутом.
func (s *Service) SetState(userID int64, stateName string, data interface{}) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()

	s.states[userID] = &AdminState{
		State:     stateName,
		Data:      data,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
}

// ClearState сбрасывает состояние диалога.
func (s *Service) ClearState(userID int64) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	delete(s.states, userID)
}

// GetUsersWithoutRole возвращает участников без роли.
func (s *Service) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return s.memberRepo.GetUsersWithoutRole(ctx)
}

// GetUsersWithRole возвращает участников с ролью.
func (s *Service) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return s.memberRepo.GetUsersWithRole(ctx)
}

// AssignRole назначает роль участнику.
func (s *Service) AssignRole(ctx context.Context, userID int64, role string) error {
	if len([]rune(role)) > 64 {
		return fmt.Errorf("роль слишком длинная (максимум 64 символа)")
	}
	return s.memberRepo.UpdateRole(ctx, userID, role)
}

// --- Криптографические утилиты ---

// verifyArgon2id проверяет пароль по хешу Argon2id.
// Формат хеша: $argon2id$v=19$m=65536,t=3,p=2$<salt_base64>$<hash_base64>
func verifyArgon2id(password, encodedHash string) bool {
	// Парсим хеш
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		log.Error("Некорректный формат хеша Argon2id")
		return false
	}

	// Извлекаем параметры
	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		log.WithError(err).Error("Ошибка парсинга параметров Argon2id")
		return false
	}

	// Декодируем соль
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		log.WithError(err).Error("Ошибка декодирования соли")
		return false
	}

	// Декодируем хеш
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		log.WithError(err).Error("Ошибка декодирования хеша")
		return false
	}

	// Вычисляем хеш введённого пароля
	computedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))

	// Сравниваем в постоянном времени (защита от timing attack)
	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}

// generateSecureToken генерирует криптографически безопасный токен сессии.
func generateSecureToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}
