// Package casino — repository.go выполняет операции с таблицами casino_games и casino_stats.
package casino

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository работает с таблицами казино в БД.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository создаёт репозиторий казино.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// SaveGame сохраняет результат спина в таблицу casino_games.
func (r *Repository) SaveGame(ctx context.Context, game *Game) error {
	query := `
		INSERT INTO casino_games (user_id, game_type, bet_amount, result_amount, game_data, rtp_percentage)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Exec(ctx, query,
		game.UserID, game.GameType, game.BetAmount,
		game.ResultAmount, game.GameData, game.RTPPercent,
	)
	if err != nil {
		return fmt.Errorf("ошибка сохранения игры: %w", err)
	}
	return nil
}

// GetStats возвращает статистику казино пользователя.
func (r *Repository) GetStats(ctx context.Context, userID int64) (*Stats, error) {
	query := `
		SELECT id, user_id, total_spins, total_wagered, total_won, biggest_win,
		       current_rtp, created_at, updated_at
		FROM casino_stats
		WHERE user_id = $1
	`
	var s Stats
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&s.ID, &s.UserID, &s.TotalSpins, &s.TotalWagered,
		&s.TotalWon, &s.BiggestWin, &s.CurrentRTP,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("статистика не найдена: %w", err)
	}
	return &s, nil
}

// CreateStats создаёт начальную статистику для пользователя.
func (r *Repository) CreateStats(ctx context.Context, userID int64, initialRTP float64) error {
	query := `
		INSERT INTO casino_stats (user_id, total_spins, total_wagered, total_won, biggest_win, current_rtp)
		VALUES ($1, 0, 0, 0, 0, $2)
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, userID, initialRTP)
	return err
}

// UpdateStats обновляет статистику после спина.
// Обновляет в одном запросе: спины, ставки, выигрыши, рекорд и RTP.
func (r *Repository) UpdateStats(ctx context.Context, userID int64, betAmount, wonAmount int64) error {
	query := `
		INSERT INTO casino_stats (user_id, total_spins, total_wagered, total_won, biggest_win, current_rtp)
		VALUES ($1, 1, $2, $3, $3,
			CASE WHEN $2 = 0 THEN 96.0 ELSE ($3::DECIMAL / $2::DECIMAL) * 100 END)
		ON CONFLICT (user_id) DO UPDATE SET
			total_spins = casino_stats.total_spins + 1,
			total_wagered = casino_stats.total_wagered + $2,
			total_won = casino_stats.total_won + $3,
			biggest_win = GREATEST(casino_stats.biggest_win, $3),
			current_rtp = CASE
				WHEN (casino_stats.total_wagered + $2) = 0 THEN 96.0
				ELSE ((casino_stats.total_won + $3)::DECIMAL / (casino_stats.total_wagered + $2)::DECIMAL) * 100
			END,
			updated_at = NOW()
	`
	_, err := r.db.Exec(ctx, query, userID, betAmount, wonAmount)
	if err != nil {
		return fmt.Errorf("ошибка обновления статистики: %w", err)
	}
	return nil
}

// GetStatsOrDefault возвращает статистику или значения по умолчанию.
func (r *Repository) GetStatsOrDefault(ctx context.Context, userID int64) *Stats {
	stats, err := r.GetStats(ctx, userID)
	if err != nil {
		return &Stats{
			UserID:     userID,
			CurrentRTP: 96.0,
		}
	}
	return stats
}

// SaveGameData сериализует данные игры в JSON для сохранения.
func SaveGameData(grid Grid, wins []WinLine, scatters int) json.RawMessage {
	data := map[string]interface{}{
		"grid":     grid,
		"wins":     wins,
		"scatters": scatters,
	}
	bytes, _ := json.Marshal(data)
	return bytes
}
