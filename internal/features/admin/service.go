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

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

// Service управляет админ-панелью.
type Service struct {
	repo       adminRepo
	memberRepo memberRepo
	cfg        *config.Config
	states     map[int64]*AdminState // Состояния диалогов (in-memory)
	panelMsgs  map[int64]int         // message_id panel-сообщения на администратора
	panelMsgAt map[int64]time.Time   // последняя активность panel message_id
	statesMu   sync.RWMutex
}

type adminRepo interface {
	CreateSession(ctx context.Context, session *AdminSession) error
	GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error)
	DeactivateSession(ctx context.Context, userID int64) error
	UpdateActivity(ctx context.Context, userID int64) error
	LogAttempt(ctx context.Context, userID int64, success bool) error
	GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error)
	ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error)
	CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error
}

type memberRepo interface {
	GetByUserID(ctx context.Context, userID int64) (*members.Member, error)
	GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error)
	GetUsersWithRole(ctx context.Context) ([]*members.Member, error)
	UpdateRole(ctx context.Context, userID int64, role string) error
	UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error
}

// NewService создаёт сервис админ-панели.
func NewService(repo adminRepo, memberRepo memberRepo, cfg *config.Config) *Service {
	return &Service{
		repo:       repo,
		memberRepo: memberRepo,
		cfg:        cfg,
		states:     make(map[int64]*AdminState),
		panelMsgs:  make(map[int64]int),
		panelMsgAt: make(map[int64]time.Time),
	}
}

// CanEnterAdmin проверяет, может ли пользователь входить в админ-поток.
// Единая точка gate-логики: позже можно заменить на permission-check.
func (s *Service) CanEnterAdmin(ctx context.Context, userID int64) bool {
	if s.isConfiguredAdmin(userID) {
		if err := s.memberRepo.UpdateAdminFlag(ctx, userID, true); err != nil {
			log.WithError(err).WithField("user_id", userID).Warn("не удалось проставить is_admin для ADMIN_IDS")
		}
		return true
	}

	member, err := s.memberRepo.GetByUserID(ctx, userID)
	if err != nil || member == nil {
		return false
	}

	return member.IsAdmin
}

func (s *Service) isConfiguredAdmin(userID int64) bool {
	for _, id := range s.cfg.AdminIDs {
		if id == userID {
			return true
		}
	}
	return false
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
	state, ok := s.states[userID]
	s.statesMu.RUnlock()
	if !ok {
		return nil
	}
	if time.Now().After(state.ExpiresAt) {
		s.ClearState(userID)
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
	delete(s.panelMsgs, userID)
	delete(s.panelMsgAt, userID)
}

// SetPanelMessageID запоминает message_id «панельного» сообщения для single-thread UI.
func (s *Service) SetPanelMessageID(userID int64, messageID int) {
	if messageID <= 0 {
		return
	}
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	s.cleanupPanelMessagesLocked()
	s.panelMsgs[userID] = messageID
	s.panelMsgAt[userID] = time.Now()
}

// GetPanelMessageID возвращает message_id «панельного» сообщения.
func (s *Service) GetPanelMessageID(userID int64) int {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()
	return s.panelMsgs[userID]
}

func (s *Service) cleanupPanelMessagesLocked() {
	cutoff := time.Now().Add(-24 * time.Hour)
	for userID, ts := range s.panelMsgAt {
		if ts.Before(cutoff) {
			delete(s.panelMsgs, userID)
			delete(s.panelMsgAt, userID)
		}
	}
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
