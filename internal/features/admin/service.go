// Package admin вЂ” service.go СЃРѕРґРµСЂР¶РёС‚ Р»РѕРіРёРєСѓ Р°СѓС‚РµРЅС‚РёС„РёРєР°С†РёРё, СѓРїСЂР°РІР»РµРЅРёСЏ СЃРµСЃСЃРёСЏРјРё
// Рё state-РјР°С€РёРЅСѓ РґР»СЏ РїРѕС€Р°РіРѕРІС‹С… Р°РґРјРёРЅ-РґРµР№СЃС‚РІРёР№.
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

// Service СѓРїСЂР°РІР»СЏРµС‚ Р°РґРјРёРЅ-РїР°РЅРµР»СЊСЋ.
type Service struct {
	repo       *Repository
	memberRepo *members.Repository
	cfg        *config.Config
	states     map[int64]*AdminState // РЎРѕСЃС‚РѕСЏРЅРёСЏ РґРёР°Р»РѕРіРѕРІ (in-memory)
	statesMu   sync.RWMutex
}

// NewService СЃРѕР·РґР°С‘С‚ СЃРµСЂРІРёСЃ Р°РґРјРёРЅ-РїР°РЅРµР»Рё.
func NewService(repo *Repository, memberRepo *members.Repository, cfg *config.Config) *Service {
	return &Service{
		repo:       repo,
		memberRepo: memberRepo,
		cfg:        cfg,
		states:     make(map[int64]*AdminState),
	}
}

// VerifyPassword РїСЂРѕРІРµСЂСЏРµС‚ РїР°СЂРѕР»СЊ Р°РґРјРёРЅРёСЃС‚СЂР°С‚РѕСЂР° СЃ РёСЃРїРѕР»СЊР·РѕРІР°РЅРёРµРј Argon2id.
// Р’РєР»СЋС‡Р°РµС‚ Р·Р°С‰РёС‚Сѓ РѕС‚ brute-force: 3 РЅРµСѓРґР°С‡РЅС‹Рµ РїРѕРїС‹С‚РєРё = Р±Р»РѕРєРёСЂРѕРІРєР° РЅР° 1 С‡Р°СЃ.
func (s *Service) VerifyPassword(ctx context.Context, userID int64, password string) error {
	// РџСЂРѕРІРµСЂСЏРµРј Р»РёРјРёС‚ РїРѕРїС‹С‚РѕРє
	attempts, err := s.repo.GetRecentAttempts(ctx, userID, 1*time.Hour)
	if err != nil {
		return err
	}
	if attempts >= 3 {
		return fmt.Errorf("СЃР»РёС€РєРѕРј РјРЅРѕРіРѕ РїРѕРїС‹С‚РѕРє, РїРѕРґРѕР¶РґРёС‚Рµ 1 С‡Р°СЃ")
	}

	// РџСЂРѕРІРµСЂСЏРµРј РїР°СЂРѕР»СЊ
	match := verifyArgon2id(password, s.cfg.AdminPasswordHash)

	// Р›РѕРіРёСЂСѓРµРј РїРѕРїС‹С‚РєСѓ
	s.repo.LogAttempt(ctx, userID, match)

	if !match {
		return fmt.Errorf("РЅРµРІРµСЂРЅС‹Р№ РїР°СЂРѕР»СЊ")
	}

	// РЎРѕР·РґР°С‘Рј СЃРµСЃСЃРёСЋ (24 С‡Р°СЃР°)
	token := generateSecureToken()
	session := &AdminSession{
		UserID:       userID,
		SessionToken: token,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	return s.repo.CreateSession(ctx, session)
}

// HasActiveSession РїСЂРѕРІРµСЂСЏРµС‚, РµСЃС‚СЊ Р»Рё Сѓ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ Р°РєС‚РёРІРЅР°СЏ СЃРµСЃСЃРёСЏ.
func (s *Service) HasActiveSession(ctx context.Context, userID int64) bool {
	session, err := s.repo.GetActiveSession(ctx, userID)
	return err == nil && session != nil
}

// GetState РІРѕР·РІСЂР°С‰Р°РµС‚ С‚РµРєСѓС‰РµРµ СЃРѕСЃС‚РѕСЏРЅРёРµ РґРёР°Р»РѕРіР°.
func (s *Service) GetState(userID int64) *AdminState {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()

	state, ok := s.states[userID]
	if !ok {
		return nil
	}
	// РџСЂРѕРІРµСЂСЏРµРј РёСЃС‚РµС‡РµРЅРёРµ
	if time.Now().After(state.ExpiresAt) {
		return nil
	}
	return state
}

// SetState СѓСЃС‚Р°РЅР°РІР»РёРІР°РµС‚ СЃРѕСЃС‚РѕСЏРЅРёРµ РґРёР°Р»РѕРіР° СЃ 5-РјРёРЅСѓС‚РЅС‹Рј С‚Р°Р№РјР°СѓС‚РѕРј.
func (s *Service) SetState(userID int64, stateName string, data interface{}) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()

	s.states[userID] = &AdminState{
		State:     stateName,
		Data:      data,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
}

// ClearState СЃР±СЂР°СЃС‹РІР°РµС‚ СЃРѕСЃС‚РѕСЏРЅРёРµ РґРёР°Р»РѕРіР°.
func (s *Service) ClearState(userID int64) {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	delete(s.states, userID)
}

// GetUsersWithoutRole РІРѕР·РІСЂР°С‰Р°РµС‚ СѓС‡Р°СЃС‚РЅРёРєРѕРІ Р±РµР· СЂРѕР»Рё.
func (s *Service) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return s.memberRepo.GetUsersWithoutRole(ctx)
}

// GetUsersWithRole РІРѕР·РІСЂР°С‰Р°РµС‚ СѓС‡Р°СЃС‚РЅРёРєРѕРІ СЃ СЂРѕР»СЊСЋ.
func (s *Service) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return s.memberRepo.GetUsersWithRole(ctx)
}

// AssignRole РЅР°Р·РЅР°С‡Р°РµС‚ СЂРѕР»СЊ СѓС‡Р°СЃС‚РЅРёРєСѓ.
func (s *Service) AssignRole(ctx context.Context, userID int64, role string) error {
	if len([]rune(role)) > 64 {
		return fmt.Errorf("СЂРѕР»СЊ СЃР»РёС€РєРѕРј РґР»РёРЅРЅР°СЏ (РјР°РєСЃРёРјСѓРј 64 СЃРёРјРІРѕР»Р°)")
	}
	return s.memberRepo.UpdateRole(ctx, userID, role)
}

// --- РљСЂРёРїС‚РѕРіСЂР°С„РёС‡РµСЃРєРёРµ СѓС‚РёР»РёС‚С‹ ---

// verifyArgon2id РїСЂРѕРІРµСЂСЏРµС‚ РїР°СЂРѕР»СЊ РїРѕ С…РµС€Сѓ Argon2id.
// Р¤РѕСЂРјР°С‚ С…РµС€Р°: $argon2id$v=19$m=65536,t=3,p=2$<salt_base64>$<hash_base64>
func verifyArgon2id(password, encodedHash string) bool {
	// РџР°СЂСЃРёРј С…РµС€
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		log.Error("РќРµРєРѕСЂСЂРµРєС‚РЅС‹Р№ С„РѕСЂРјР°С‚ С…РµС€Р° Argon2id")
		return false
	}

	// РР·РІР»РµРєР°РµРј РїР°СЂР°РјРµС‚СЂС‹
	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РїР°СЂСЃРёРЅРіР° РїР°СЂР°РјРµС‚СЂРѕРІ Argon2id")
		return false
	}

	// Р”РµРєРѕРґРёСЂСѓРµРј СЃРѕР»СЊ
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РґРµРєРѕРґРёСЂРѕРІР°РЅРёСЏ СЃРѕР»Рё")
		return false
	}

	// Р”РµРєРѕРґРёСЂСѓРµРј С…РµС€
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РґРµРєРѕРґРёСЂРѕРІР°РЅРёСЏ С…РµС€Р°")
		return false
	}

	// Р’С‹С‡РёСЃР»СЏРµРј С…РµС€ РІРІРµРґС‘РЅРЅРѕРіРѕ РїР°СЂРѕР»СЏ
	computedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))

	// РЎСЂР°РІРЅРёРІР°РµРј РІ РїРѕСЃС‚РѕСЏРЅРЅРѕРј РІСЂРµРјРµРЅРё (Р·Р°С‰РёС‚Р° РѕС‚ timing attack)
	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}

// generateSecureToken РіРµРЅРµСЂРёСЂСѓРµС‚ РєСЂРёРїС‚РѕРіСЂР°С„РёС‡РµСЃРєРё Р±РµР·РѕРїР°СЃРЅС‹Р№ С‚РѕРєРµРЅ СЃРµСЃСЃРёРё.
func generateSecureToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}
