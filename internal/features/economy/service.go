// Package economy вЂ” service.go СЃРѕРґРµСЂР¶РёС‚ Р±РёР·РЅРµСЃ-Р»РѕРіРёРєСѓ СЌРєРѕРЅРѕРјРёРєРё.
// Р’Р°Р»РёРґР°С†РёСЏ, РїРµСЂРµРІРѕРґС‹, РїРѕР»СѓС‡РµРЅРёРµ Р±Р°Р»Р°РЅСЃР° Рё РёСЃС‚РѕСЂРёРё С‚СЂР°РЅР·Р°РєС†РёР№.
package economy

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
)

// Service СѓРїСЂР°РІР»СЏРµС‚ СЌРєРѕРЅРѕРјРёРєРѕР№ Р±РѕС‚Р° (РїР»РµРЅРєРё).
type Service struct {
	repo *Repository // Р РµРїРѕР·РёС‚РѕСЂРёР№ РґР»СЏ СЂР°Р±РѕС‚С‹ СЃ Р‘Р”
}

// NewService СЃРѕР·РґР°С‘С‚ РЅРѕРІС‹Р№ СЃРµСЂРІРёСЃ СЌРєРѕРЅРѕРјРёРєРё.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetBalance РІРѕР·РІСЂР°С‰Р°РµС‚ С‚РµРєСѓС‰РёР№ Р±Р°Р»Р°РЅСЃ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ.
func (s *Service) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return s.repo.GetBalance(ctx, userID)
}

// AddBalance РЅР°С‡РёСЃР»СЏРµС‚ РїР»РµРЅРєРё РїРѕР»СЊР·РѕРІР°С‚РµР»СЋ.
// РСЃРїРѕР»СЊР·СѓРµС‚СЃСЏ РґР»СЏ Р±РѕРЅСѓСЃРѕРІ СЃС‚СЂРёРєРѕРІ, РІС‹РёРіСЂС‹С€РµР№ РєР°Р·РёРЅРѕ Рё С‚.Рґ.
func (s *Service) AddBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	if amount <= 0 {
		return common.ErrInvalidAmount
	}
	return s.repo.AddBalance(ctx, userID, amount, txType, description)
}

// DeductBalance СЃРїРёСЃС‹РІР°РµС‚ РїР»РµРЅРєРё.
// РСЃРїРѕР»СЊР·СѓРµС‚СЃСЏ РґР»СЏ СЃС‚Р°РІРѕРє РєР°Р·РёРЅРѕ Рё РґСЂСѓРіРёС… С‚СЂР°С‚.
func (s *Service) DeductBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	if amount <= 0 {
		return common.ErrInvalidAmount
	}
	return s.repo.DeductBalance(ctx, userID, amount, txType, description)
}

// Transfer РїРµСЂРµРІРѕРґРёС‚ РїР»РµРЅРєРё РѕС‚ РѕРґРЅРѕРіРѕ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ Рє РґСЂСѓРіРѕРјСѓ.
// Р’С‹РїРѕР»РЅСЏРµС‚ РІСЃРµ РЅРµРѕР±С…РѕРґРёРјС‹Рµ РїСЂРѕРІРµСЂРєРё:
//   - РќРµР»СЊР·СЏ РїРµСЂРµРІРѕРґРёС‚СЊ СЃРµР±Рµ
//   - РЎСѓРјРјР° РґРѕР»Р¶РЅР° Р±С‹С‚СЊ РїРѕР»РѕР¶РёС‚РµР»СЊРЅРѕР№
//   - РЈ РѕС‚РїСЂР°РІРёС‚РµР»СЏ РґРѕР»Р¶РЅРѕ Р±С‹С‚СЊ РґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїР»РµРЅРѕРє
func (s *Service) Transfer(ctx context.Context, fromUserID, toUserID, amount int64) error {
	// РџСЂРѕРІРµСЂРєР°: РЅРµР»СЊР·СЏ РѕС‚РїСЂР°РІРёС‚СЊ СЃРµР±Рµ
	if fromUserID == toUserID {
		return common.ErrSelfTransfer
	}

	// РџСЂРѕРІРµСЂРєР°: СЃСѓРјРјР° РґРѕР»Р¶РЅР° Р±С‹С‚СЊ РїРѕР»РѕР¶РёС‚РµР»СЊРЅРѕР№
	if amount <= 0 {
		return common.ErrInvalidAmount
	}

	// Р’С‹РїРѕР»РЅСЏРµРј РїРµСЂРµРІРѕРґ (РїСЂРѕРІРµСЂРєР° Р±Р°Р»Р°РЅСЃР° РІРЅСѓС‚СЂРё СЂРµРїРѕР·РёС‚РѕСЂРёСЏ)
	err := s.repo.Transfer(ctx, fromUserID, toUserID, amount)
	if err != nil {
		// Р•СЃР»Рё РѕС€РёР±РєР° СЃРѕРґРµСЂР¶РёС‚ "РЅРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ" вЂ” СЌС‚Рѕ РЅРµС…РІР°С‚РєР° РїР»РµРЅРѕРє
		if strings.Contains(err.Error(), "РЅРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ") {
			return common.ErrInsufficientBalance
		}
		return err
	}

	log.WithFields(log.Fields{
		"from": fromUserID,
		"to":   toUserID,
		"amount": amount,
	}).Info("РџРµСЂРµРІРѕРґ РІС‹РїРѕР»РЅРµРЅ")

	return nil
}

// GetTransactionHistory РІРѕР·РІСЂР°С‰Р°РµС‚ С„РѕСЂРјР°С‚РёСЂРѕРІР°РЅРЅСѓСЋ РёСЃС‚РѕСЂРёСЋ С‚СЂР°РЅР·Р°РєС†РёР№.
// РџРѕСЃР»РµРґРЅРёРµ 10 С‚СЂР°РЅР·Р°РєС†РёР№. Р•СЃР»Рё Р±РѕР»СЊС€Рµ 5 вЂ” РѕР±РѕСЂР°С‡РёРІР°РµС‚ РІ СЃРїРѕР№Р»РµСЂ.
func (s *Service) GetTransactionHistory(ctx context.Context, userID int64) (string, error) {
	transactions, err := s.repo.GetTransactions(ctx, userID, 10)
	if err != nil {
		return "", err
	}

	if len(transactions) == 0 {
		return "рџ“‹ РЈ РІР°СЃ РїРѕРєР° РЅРµС‚ С‚СЂР°РЅР·Р°РєС†РёР№", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("рџ“‹ РџРѕСЃР»РµРґРЅРёРµ %d С‚СЂР°РЅР·Р°РєС†РёР№:\n\n", len(transactions)))

	// Р¤РѕСЂРјРёСЂСѓРµРј СЃС‚СЂРѕРєРё С‚СЂР°РЅР·Р°РєС†РёР№
	var lines []string
	for i, tx := range transactions {
		// РћРїСЂРµРґРµР»СЏРµРј Р·РЅР°Рє: + РµСЃР»Рё РїРѕР»СѓС‡РёР»Рё, - РµСЃР»Рё РѕС‚РїСЂР°РІРёР»Рё
		sign := "+"
		if tx.FromUserID != nil && *tx.FromUserID == userID {
			sign = "-"
		}

		line := fmt.Sprintf("%d. %s | %s%d %s | %s",
			i+1,
			common.FormatDateTime(tx.CreatedAt),
			sign,
			tx.Amount,
			common.PluralizeFilms(tx.Amount),
			tx.Description,
		)
		lines = append(lines, line)
	}

	// Р•СЃР»Рё Р±РѕР»СЊС€Рµ 5 вЂ” РѕР±РѕСЂР°С‡РёРІР°РµРј РІ СЃРїРѕР№Р»РµСЂ (||С‚РµРєСЃС‚||)
	if len(lines) > 5 {
		// РџРµСЂРІС‹Рµ 5 РїРѕРєР°Р·С‹РІР°РµРј РѕС‚РєСЂС‹С‚Рѕ
		for _, line := range lines[:5] {
			sb.WriteString(line + "\n")
		}
		// РћСЃС‚Р°Р»СЊРЅС‹Рµ РІ СЃРїРѕР№Р»РµСЂРµ
		sb.WriteString("\n||")
		for _, line := range lines[5:] {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("||")
	} else {
		for _, line := range lines {
			sb.WriteString(line + "\n")
		}
	}

	return sb.String(), nil
}

// CreateBalance СЃРѕР·РґР°С‘С‚ РЅР°С‡Р°Р»СЊРЅС‹Р№ Р±Р°Р»Р°РЅСЃ РґР»СЏ РЅРѕРІРѕРіРѕ СѓС‡Р°СЃС‚РЅРёРєР° (0 РїР»РµРЅРѕРє).
func (s *Service) CreateBalance(ctx context.Context, userID int64) error {
	return s.repo.EnsureBalance(ctx, userID)
}
