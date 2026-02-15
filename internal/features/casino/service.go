// Package casino — service.go координирует спин слотов от начала до конца.
package casino

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/config"
	"telegram-bot/internal/features/economy"
)

// Service управляет казино.
type Service struct {
	repo           *Repository
	economyService *economy.Service
	rtpManager     *RTPManager
	cfg            *config.Config
}

// NewService создаёт сервис казино.
func NewService(repo *Repository, economyService *economy.Service, cfg *config.Config) *Service {
	return &Service{
		repo:           repo,
		economyService: economyService,
		rtpManager:     NewRTPManager(cfg.CasinoMinRTP, cfg.CasinoMaxRTP, cfg.CasinoInitRTP),
		cfg:            cfg,
	}
}

// PlaySlots выполняет полный цикл спина.
func (s *Service) PlaySlots(ctx context.Context, userID int64) (*SlotResult, error) {
	bet := s.cfg.CasinoSlotsBet

	// Проверяем баланс
	balance, err := s.economyService.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения баланса: %w", err)
	}
	if balance < bet {
		return nil, fmt.Errorf("недостаточно пленок: нужно %d, у тебя %d", bet, balance)
	}

	// Списываем ставку
	err = s.economyService.DeductBalance(ctx, userID, bet, "casino_bet", "Slots bet")
	if err != nil {
		return nil, fmt.Errorf("ошибка списания ставки: %w", err)
	}

	// Генерируем сетку с учётом RTP
	symbols := s.rtpManager.GetAdjustedWeights(userID)
	grid, err := GenerateGrid(symbols)
	if err != nil {
		_ = s.economyService.AddBalance(ctx, userID, bet, "casino_refund", "Bet refund")
		return nil, fmt.Errorf("ошибка генерации: %w", err)
	}

	// Проверяем линии и скаттеры
	winLines := CheckPaylines(grid, bet)
	scatterCount := CountScatters(grid)
	scatterBonus, freeSpins := CalculateScatterBonus(scatterCount)

	var totalPayout int64
	for _, win := range winLines {
		totalPayout += win.Payout
	}
	totalPayout += scatterBonus

	// Фриспины
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

	// Начисляем выигрыш
	if totalPayout > 0 {
		err = s.economyService.AddBalance(ctx, userID, totalPayout, "casino_win", "Slots win")
		if err != nil {
			log.WithError(err).Error("Ошибка начисления выигрыша")
		}
	}

	// Обновляем статистику
	if err := s.repo.UpdateStats(ctx, userID, bet, totalPayout); err != nil {
		log.WithError(err).Error("Ошибка обновления статистики казино")
	}

	// Корректируем RTP
	stats := s.repo.GetStatsOrDefault(ctx, userID)
	s.rtpManager.AdjustRTP(userID, stats.CurrentRTP)

	// Сохраняем игру
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
		log.WithError(err).Error("Ошибка сохранения игры")
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

// GetStats возвращает статистику казино пользователя.
func (s *Service) GetStats(ctx context.Context, userID int64) (*Stats, error) {
	return s.repo.GetStats(ctx, userID)
}
