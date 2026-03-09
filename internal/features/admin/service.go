package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/argon2"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type Service struct {
	repo        adminRepo
	memberRepo  memberRepo
	cfg         *config.Config
	riddles     *RiddleService
	permissions *permissionSet
}

type adminRepo interface {
	CreateSession(ctx context.Context, session *AdminSession) error
	GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error)
	DeactivateSession(ctx context.Context, userID int64) error
	UpdateActivity(ctx context.Context, userID int64) error
	LogAttempt(ctx context.Context, userID int64, success bool) error
	GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error)
	CleanupStaleAuthState(ctx context.Context, now time.Time) (CleanupResult, error)
	ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error)
	CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error
	DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error
	SaveFlowState(ctx context.Context, userID int64, state *AdminState) error
	GetFlowState(ctx context.Context, userID int64) (*AdminState, error)
	ClearFlowState(ctx context.Context, userID int64) error
	SetPanelMessage(ctx context.Context, userID int64, panel AdminPanelMessage) error
	GetPanelMessage(ctx context.Context, userID int64) (AdminPanelMessage, error)
	ClearPanelMessage(ctx context.Context, userID int64) error
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
		repo:        repo,
		memberRepo:  memberRepo,
		cfg:         cfg,
		permissions: newPermissionSet(cfg),
	}
}

func (s *Service) SetRiddleService(riddles *RiddleService) {
	s.riddles = riddles
}

func (s *Service) CanEnterAdmin(ctx context.Context, userID int64) bool {
	if s.permissions.isEnvAdmin(userID) && s.memberRepo != nil {
		if err := s.memberRepo.UpdateAdminFlag(ctx, userID, true); err != nil {
			log.WithError(err).WithField("user_id", userID).Warn("не удалось проставить is_admin для ADMIN_IDS")
		}
	}
	return s.CanAccessAdminPanel(ctx, userID)
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
	state, err := s.repo.GetFlowState(context.Background(), userID)
	if err != nil || state == nil {
		return nil
	}
	if time.Now().After(state.ExpiresAt) {
		s.ClearState(userID)
		return nil
	}
	return state
}

func (s *Service) SetState(userID int64, stateName string, data interface{}) {
	state := &AdminState{
		State:     stateName,
		Data:      data,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	if err := s.repo.SaveFlowState(context.Background(), userID, state); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("не удалось сохранить admin flow state")
	}
}

func (s *Service) ClearState(userID int64) {
	if err := s.repo.ClearFlowState(context.Background(), userID); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("не удалось очистить admin flow state")
	}
}

func (s *Service) ClearPanelMessage(userID int64) {
	if err := s.repo.ClearPanelMessage(context.Background(), userID); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("не удалось очистить admin panel message")
	}
}

func (s *Service) SetPanelMessage(userID, chatID int64, messageID int) {
	if chatID == 0 || messageID <= 0 {
		return
	}
	if err := s.repo.SetPanelMessage(context.Background(), userID, AdminPanelMessage{ChatID: chatID, MessageID: messageID}); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("не удалось сохранить admin panel message")
	}
}

func (s *Service) SetPanelMessageID(userID int64, messageID int) {
	s.SetPanelMessage(userID, userID, messageID)
}

func (s *Service) GetPanelMessage(userID int64) AdminPanelMessage {
	panel, err := s.repo.GetPanelMessage(context.Background(), userID)
	if err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("не удалось получить admin panel message")
		return AdminPanelMessage{}
	}
	return panel
}

func (s *Service) GetPanelMessageID(userID int64) int {
	return s.GetPanelMessage(userID).MessageID
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

func (s *Service) CleanupStaleAuthState(ctx context.Context, now time.Time) error {
	_, err := s.repo.CleanupStaleAuthState(ctx, now)
	if err != nil {
		return fmt.Errorf("cleanup admin auth state: %w", err)
	}
	if s.riddles != nil {
		if err := s.riddles.CleanupExpired(ctx, now); err != nil {
			return fmt.Errorf("cleanup riddles: %w", err)
		}
	}
	return nil
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
