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

type Service struct {
	repo       adminRepo
	memberRepo memberRepo
	cfg        *config.Config
	states     map[int64]*AdminState
	panelMsgs  map[int64]AdminPanelMessage
	panelMsgAt map[int64]time.Time
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
	DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error
}

type memberRepo interface {
	GetByUserID(ctx context.Context, userID int64) (*members.Member, error)
	GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error)
	GetUsersWithRole(ctx context.Context) ([]*members.Member, error)
	UpdateRole(ctx context.Context, userID int64, role string) error
	UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error
}

func NewService(repo adminRepo, memberRepo memberRepo, cfg *config.Config) *Service {
	return &Service{
		repo:       repo,
		memberRepo: memberRepo,
		cfg:        cfg,
		states:     make(map[int64]*AdminState),
		panelMsgs:  make(map[int64]AdminPanelMessage),
		panelMsgAt: make(map[int64]time.Time),
	}
}

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

func (s *Service) VerifyPassword(ctx context.Context, userID int64, password string) error {
	attempts, err := s.repo.GetRecentAttempts(ctx, userID, 1*time.Hour)
	if err != nil {
		return err
	}
	if attempts >= 3 {
		return fmt.Errorf("слишком много попыток, подождите 1 час")
	}

	match := verifyArgon2id(password, s.cfg.AdminPasswordHash)
	if err := s.repo.LogAttempt(ctx, userID, match); err != nil {
		log.WithError(err).WithFields(log.Fields{"user_id": userID, "success": match}).Warn("не удалось сохранить попытку входа администратора")
	}
	if !match {
		return fmt.Errorf("неверный пароль")
	}

	token := generateSecureToken()
	session := &AdminSession{
		UserID:       userID,
		SessionToken: token,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	return s.repo.CreateSession(ctx, session)
}

func (s *Service) HasActiveSession(ctx context.Context, userID int64) bool {
	session, err := s.repo.GetActiveSession(ctx, userID)
	return err == nil && session != nil
}

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

func (s *Service) SetState(userID int64, stateName string, data interface{}) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()

	s.states[userID] = &AdminState{
		State:     stateName,
		Data:      data,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
}

func (s *Service) ClearState(userID int64) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	delete(s.states, userID)
	delete(s.panelMsgs, userID)
	delete(s.panelMsgAt, userID)
}

func (s *Service) ClearPanelMessage(userID int64) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	delete(s.panelMsgs, userID)
	delete(s.panelMsgAt, userID)
}

func (s *Service) SetPanelMessage(userID, chatID int64, messageID int) {
	if chatID == 0 || messageID <= 0 {
		return
	}
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	s.cleanupPanelMessagesLocked()
	s.panelMsgs[userID] = AdminPanelMessage{ChatID: chatID, MessageID: messageID}
	s.panelMsgAt[userID] = time.Now()
}

func (s *Service) SetPanelMessageID(userID int64, messageID int) {
	s.SetPanelMessage(userID, userID, messageID)
}

func (s *Service) GetPanelMessage(userID int64) AdminPanelMessage {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()
	return s.panelMsgs[userID]
}

func (s *Service) GetPanelMessageID(userID int64) int {
	return s.GetPanelMessage(userID).MessageID
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

func (s *Service) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return s.memberRepo.GetUsersWithoutRole(ctx)
}

func (s *Service) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return s.memberRepo.GetUsersWithRole(ctx)
}

func (s *Service) AssignRole(ctx context.Context, userID int64, role string) error {
	if len([]rune(role)) > 64 {
		return fmt.Errorf("роль слишком длинная (максимум 64 символа)")
	}
	return s.memberRepo.UpdateRole(ctx, userID, role)
}

func (s *Service) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
	if chatID <= 0 || deltaID <= 0 {
		return fmt.Errorf("некорректные параметры удаления дельты")
	}
	return s.repo.DeleteBalanceDelta(ctx, chatID, deltaID)
}

func verifyArgon2id(password, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		log.Error("Некорректный формат хеша Argon2id")
		return false
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		log.WithError(err).Error("Ошибка парсинга параметров Argon2id")
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		log.WithError(err).Error("Ошибка декодирования соли")
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		log.WithError(err).Error("Ошибка декодирования хеша")
		return false
	}

	computedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}

func generateSecureToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}
