// Package streak — service.go содержит основную бизнес-логику стрик-системы.
// Сервис считает сообщения, начисляет бонусы и управляет ежедневным сбросом.
package streak

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/common"
	"telegram-bot/internal/config"
	"telegram-bot/internal/features/economy"
)

// Service управляет стрик-системой.
type Service struct {
	repo           *Repository      // Репозиторий стриков
	economyService *economy.Service // Сервис экономики для начисления бонусов
	cfg            *config.Config   // Конфигурация
}

// NewService создаёт новый сервис стриков.
func NewService(repo *Repository, economyService *economy.Service, cfg *config.Config) *Service {
	return &Service{
		repo:           repo,
		economyService: economyService,
		cfg:            cfg,
	}
}

// CountMessage обрабатывает входящее сообщение для подсчёта стрика.
// Вызывается для КАЖДОГО сообщения в FLOOD_CHAT_ID.
//
// Алгоритм:
//  1. Проверяем, содержит ли сообщение 3+ слов
//  2. Проверяем, не является ли командой
//  3. Проверяем, не выполнена ли уже норма
//  4. Увеличиваем счётчик
//  5. Если достигнута норма (50) — начисляем бонус МОЛЧА
func (s *Service) CountMessage(ctx context.Context, userID int64, text string) error {
	// Шаг 1-2: Проверяем, подходит ли сообщение
	if !IsValidForStreak(text) {
		return nil // Сообщение не подходит — игнорируем
	}

	// Шаг 3: Получаем текущий стрик
	streak, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		// Если стрик не найден — создаём (первое сообщение после регистрации)
		if err := s.repo.Create(ctx, userID); err != nil {
			return err
		}
		streak, err = s.repo.GetByUserID(ctx, userID)
		if err != nil {
			return err
		}
	}

	// Если норма уже выполнена — не считаем дальше
	if streak.QuotaCompletedToday {
		return nil
	}

	// Шаг 4: Увеличиваем счётчик
	updated, err := s.repo.IncrementMessages(ctx, userID)
	if err != nil {
		return err
	}

	// Шаг 5: Проверяем, достигнута ли норма
	if updated.MessagesToday >= s.cfg.StreakMessagesNeed {
		return s.completeQuota(ctx, userID, updated)
	}

	return nil
}

// completeQuota выполняет норму дня: увеличивает стрик, начисляет бонус.
// Бонус начисляется МОЛЧА — пользователь не получает уведомление.
func (s *Service) completeQuota(ctx context.Context, userID int64, streak *Streak) error {
	// Рассчитываем бонус на основе текущего стрика
	bonus := CalculateReward(streak.CurrentStreak)
	newStreak := streak.CurrentStreak + 1
	longestStreak := streak.LongestStreak
	if newStreak > longestStreak {
		longestStreak = newStreak
	}
	totalCompleted := streak.TotalQuotasCompleted + 1
	quotaDate := common.GetMoscowDate()

	// Обновляем стрик в БД
	if err := s.repo.CompleteQuota(ctx, userID, newStreak, longestStreak, totalCompleted, quotaDate); err != nil {
		return fmt.Errorf("ошибка завершения нормы: %w", err)
	}

	// Начисляем бонус МОЛЧА (без уведомления)
	description := fmt.Sprintf("Streak bonus - Day %d", newStreak)
	if err := s.economyService.AddBalance(ctx, userID, bonus, "streak_bonus", description); err != nil {
		log.WithError(err).WithField("user_id", userID).Error("Ошибка начисления стрик-бонуса")
		return err
	}

	log.WithFields(log.Fields{
		"user_id": userID,
		"day":     newStreak,
		"bonus":   bonus,
	}).Debug("Стрик-бонус начислен (молча)")

	return nil
}

// GetStreak возвращает информацию о стрике пользователя.
func (s *Service) GetStreak(ctx context.Context, userID int64) (*Streak, error) {
	return s.repo.GetByUserID(ctx, userID)
}

// CreateStreak создаёт начальную запись стрика.
func (s *Service) CreateStreak(ctx context.Context, userID int64) error {
	return s.repo.Create(ctx, userID)
}

// DailyReset сбрасывает дневные счётчики и ломает стрики тех, кто не выполнил норму.
// Запускается кроном в 00:00 по Москве.
func (s *Service) DailyReset(ctx context.Context) error {
	log.Info("Запуск ежедневного сброса стриков")

	// Получаем все стрики
	streaks, err := s.repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("ошибка получения стриков: %w", err)
	}

	brokenCount := 0
	for _, streak := range streaks {
		// Если норма НЕ была выполнена вчера и стрик > 0 — ломаем
		if !streak.QuotaCompletedToday && streak.CurrentStreak > 0 {
			if err := s.repo.BreakStreak(ctx, streak.UserID); err != nil {
				log.WithError(err).WithField("user_id", streak.UserID).Error("Ошибка сброса стрика")
			}
			brokenCount++
		}
	}

	// Сбрасываем дневные счётчики у всех
	if err := s.repo.ResetDaily(ctx); err != nil {
		return fmt.Errorf("ошибка сброса дневных счётчиков: %w", err)
	}

	log.WithFields(log.Fields{
		"total":  len(streaks),
		"broken": brokenCount,
	}).Info("Ежедневный сброс завершён")

	return nil
}

// SendReminders отправляет напоминания пользователям с длинными стриками.
// Запускается кроном каждый час.
func (s *Service) SendReminders(ctx context.Context, sendFunc func(userID int64, text string)) error {
	// Находим пользователей со стриком >= порога (по умолчанию 7)
	longStreaks, err := s.repo.GetByMinStreak(ctx, s.cfg.StreakReminderThreshold)
	if err != nil {
		return err
	}

	for _, streak := range longStreaks {
		// Пропускаем если норма уже выполнена или напоминание уже отправлено
		if streak.QuotaCompletedToday || streak.ReminderSentToday {
			continue
		}

		// Проверяем неактивность (10+ часов без сообщений)
		if streak.LastMessageAt != nil {
			inactiveHours := common.GetMoscowTime().Sub(*streak.LastMessageAt).Hours()
			if inactiveHours < float64(s.cfg.StreakInactiveHours) {
				continue // Ещё не прошло достаточно времени
			}
		}

		// Отправляем напоминание
		msg := fmt.Sprintf("⚠️ У тебя огонек %d %s! Не забудь написать сообщения, чтобы не потерять прогресс!",
			streak.CurrentStreak, common.PluralizeDays(streak.CurrentStreak))
		sendFunc(streak.UserID, msg)

		// Помечаем, что напоминание отправлено
		s.repo.MarkReminderSent(ctx, streak.UserID)
	}

	return nil
}
