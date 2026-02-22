// Package casino вЂ” service.go РєРѕРѕСЂРґРёРЅРёСЂСѓРµС‚ СЃРїРёРЅ СЃР»РѕС‚РѕРІ РѕС‚ РЅР°С‡Р°Р»Р° РґРѕ РєРѕРЅС†Р°.
package casino

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/economy"
)

// Service СѓРїСЂР°РІР»СЏРµС‚ РєР°Р·РёРЅРѕ.
type Service struct {
	repo           *Repository
	economyService *economy.Service
	rtpManager     *RTPManager
	cfg            *config.Config
}

// NewService СЃРѕР·РґР°С‘С‚ СЃРµСЂРІРёСЃ РєР°Р·РёРЅРѕ.
func NewService(repo *Repository, economyService *economy.Service, cfg *config.Config) *Service {
	return &Service{
		repo:           repo,
		economyService: economyService,
		rtpManager:     NewRTPManager(cfg.CasinoMinRTP, cfg.CasinoMaxRTP, cfg.CasinoInitRTP),
		cfg:            cfg,
	}
}

// PlaySlots РІС‹РїРѕР»РЅСЏРµС‚ РїРѕР»РЅС‹Р№ С†РёРєР» СЃРїРёРЅР°.
func (s *Service) PlaySlots(ctx context.Context, userID int64) (*SlotResult, error) {
	bet := s.cfg.CasinoSlotsBet

	// РџСЂРѕРІРµСЂСЏРµРј Р±Р°Р»Р°РЅСЃ
	balance, err := s.economyService.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("РѕС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ Р±Р°Р»Р°РЅСЃР°: %w", err)
	}
	if balance < bet {
		return nil, fmt.Errorf("РЅРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїР»РµРЅРѕРє: РЅСѓР¶РЅРѕ %d, Сѓ С‚РµР±СЏ %d", bet, balance)
	}

	// РЎРїРёСЃС‹РІР°РµРј СЃС‚Р°РІРєСѓ
	err = s.economyService.DeductBalance(ctx, userID, bet, "casino_bet", "Slots bet")
	if err != nil {
		return nil, fmt.Errorf("РѕС€РёР±РєР° СЃРїРёСЃР°РЅРёСЏ СЃС‚Р°РІРєРё: %w", err)
	}

	// Р“РµРЅРµСЂРёСЂСѓРµРј СЃРµС‚РєСѓ СЃ СѓС‡С‘С‚РѕРј RTP
	symbols := s.rtpManager.GetAdjustedWeights(userID)
	grid, err := GenerateGrid(symbols)
	if err != nil {
		_ = s.economyService.AddBalance(ctx, userID, bet, "casino_refund", "Bet refund")
		return nil, fmt.Errorf("РѕС€РёР±РєР° РіРµРЅРµСЂР°С†РёРё: %w", err)
	}

	// РџСЂРѕРІРµСЂСЏРµРј Р»РёРЅРёРё Рё СЃРєР°С‚С‚РµСЂС‹
	winLines := CheckPaylines(grid, bet)
	scatterCount := CountScatters(grid)
	scatterBonus, freeSpins := CalculateScatterBonus(scatterCount)

	var totalPayout int64
	for _, win := range winLines {
		totalPayout += win.Payout
	}
	totalPayout += scatterBonus

	// Р¤СЂРёСЃРїРёРЅС‹
	for i := 0; i < freeSpins; i++ {
		freeGrid, err := GenerateGrid(symbols)
		if err != nil {
			break
		}
		freeWins := CheckPaylines(freeGrid, bet)
		for _, win := range freeWins {
			totalPayout += win.Payout
		}
		fb, _ := CalculateScatterBonus(CountScatters(freeGrid))
		totalPayout += fb
	}

	// РќР°С‡РёСЃР»СЏРµРј РІС‹РёРіСЂС‹С€
	if totalPayout > 0 {
		err = s.economyService.AddBalance(ctx, userID, totalPayout, "casino_win", "Slots win")
		if err != nil {
			log.WithError(err).Error("РћС€РёР±РєР° РЅР°С‡РёСЃР»РµРЅРёСЏ РІС‹РёРіСЂС‹С€Р°")
		}
	}

	// РћР±РЅРѕРІР»СЏРµРј СЃС‚Р°С‚РёСЃС‚РёРєСѓ
	if err := s.repo.UpdateStats(ctx, userID, bet, totalPayout); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РѕР±РЅРѕРІР»РµРЅРёСЏ СЃС‚Р°С‚РёСЃС‚РёРєРё РєР°Р·РёРЅРѕ")
	}

	// РљРѕСЂСЂРµРєС‚РёСЂСѓРµРј RTP
	stats := s.repo.GetStatsOrDefault(ctx, userID)
	s.rtpManager.AdjustRTP(userID, stats.CurrentRTP)

	// РЎРѕС…СЂР°РЅСЏРµРј РёРіСЂСѓ
	gameData := SaveGameData(grid, winLines, scatterCount)
	game := &Game{
		UserID:       userID,
		GameType:     "slots",
		BetAmount:    bet,
		ResultAmount: totalPayout,
		GameData:     gameData,
		RTPPercent:   stats.CurrentRTP,
	}
	if err := s.repo.SaveGame(ctx, game); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° СЃРѕС…СЂР°РЅРµРЅРёСЏ РёРіСЂС‹")
	}

	return &SlotResult{
		Grid:         grid,
		WinLines:     winLines,
		ScatterCount: scatterCount,
		ScatterWin:   scatterBonus,
		TotalPayout:  totalPayout,
		IsWin:        totalPayout > 0,
		FreeSpins:    freeSpins,
	}, nil
}

// GetStats РІРѕР·РІСЂР°С‰Р°РµС‚ СЃС‚Р°С‚РёСЃС‚РёРєСѓ РєР°Р·РёРЅРѕ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ.
func (s *Service) GetStats(ctx context.Context, userID int64) (*Stats, error) {
	return s.repo.GetStats(ctx, userID)
}
