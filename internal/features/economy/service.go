// Package economy — service.go содержит бизнес-логику экономики.
// Валидация, переводы, получение баланса и истории транзакций.
package economy

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/common"
)

// Service управляет экономикой бота (пленки).
type Service struct {
	repo *Repository // Репозиторий для работы с БД
}

// NewService создаёт новый сервис экономики.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetBalance возвращает текущий баланс пользователя.
func (s *Service) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return s.repo.GetBalance(ctx, userID)
}

// AddBalance начисляет пленки пользователю.
// Используется для бонусов стриков, выигрышей казино и т.д.
func (s *Service) AddBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	if amount <= 0 {
		return common.ErrInvalidAmount
	}
	return s.repo.AddBalance(ctx, userID, amount, txType, description)
}

// DeductBalance списывает пленки.
// Используется для ставок казино и других трат.
func (s *Service) DeductBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	if amount <= 0 {
		return common.ErrInvalidAmount
	}
	return s.repo.DeductBalance(ctx, userID, amount, txType, description)
}

// Transfer переводит пленки от одного пользователя к другому.
// Выполняет все необходимые проверки:
//   - Нельзя переводить себе
//   - Сумма должна быть положительной
//   - У отправителя должно быть достаточно пленок
func (s *Service) Transfer(ctx context.Context, fromUserID, toUserID, amount int64) error {
	// Проверка: нельзя отправить себе
	if fromUserID == toUserID {
		return common.ErrSelfTransfer
	}

	// Проверка: сумма должна быть положительной
	if amount <= 0 {
		return common.ErrInvalidAmount
	}

	// Выполняем перевод (проверка баланса внутри репозитория)
	err := s.repo.Transfer(ctx, fromUserID, toUserID, amount)
	if err != nil {
		// Если ошибка содержит "недостаточно" — это нехватка пленок
		if strings.Contains(err.Error(), "недостаточно") {
			return common.ErrInsufficientBalance
		}
		return err
	}

	log.WithFields(log.Fields{
		"from": fromUserID,
		"to":   toUserID,
		"amount": amount,
	}).Info("Перевод выполнен")

	return nil
}

// GetTransactionHistory возвращает форматированную историю транзакций.
// Последние 10 транзакций. Если больше 5 — оборачивает в спойлер.
func (s *Service) GetTransactionHistory(ctx context.Context, userID int64) (string, error) {
	transactions, err := s.repo.GetTransactions(ctx, userID, 10)
	if err != nil {
		return "", err
	}

	if len(transactions) == 0 {
		return "📋 У вас пока нет транзакций", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 Последние %d транзакций:\n\n", len(transactions)))

	// Формируем строки транзакций
	var lines []string
	for i, tx := range transactions {
		// Определяем знак: + если получили, - если отправили
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

	// Если больше 5 — оборачиваем в спойлер (||текст||)
	if len(lines) > 5 {
		// Первые 5 показываем открыто
		for _, line := range lines[:5] {
			sb.WriteString(line + "\n")
		}
		// Остальные в спойлере
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

// CreateBalance создаёт начальный баланс для нового участника (0 пленок).
func (s *Service) CreateBalance(ctx context.Context, userID int64) error {
	return s.repo.EnsureBalance(ctx, userID)
}
