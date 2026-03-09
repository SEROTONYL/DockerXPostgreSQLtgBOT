package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const riddleAdvisoryLockKey int64 = 9152026

type riddleQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type RiddleRepository struct {
	db *pgxpool.Pool
}

func NewRiddleRepository(db *pgxpool.Pool) *RiddleRepository {
	return &RiddleRepository{db: db}
}

func (r *RiddleRepository) WithTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) (err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin riddle tx: %w", err)
	}
	defer rollbackRiddleOnFailure(ctx, tx, &err)
	if err = fn(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit riddle tx: %w", err)
	}
	err = nil
	return nil
}

func (r *RiddleRepository) lockTx(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, riddleAdvisoryLockKey); err != nil {
		return fmt.Errorf("lock riddle advisory tx: %w", err)
	}
	return nil
}

func (r *RiddleRepository) CreatePublishingRiddleTx(ctx context.Context, tx pgx.Tx, adminID int64, postText string, reward int64, answers []RiddleDraftAnswer, now time.Time) (*Riddle, []*RiddleAnswer, error) {
	if err := r.lockTx(ctx, tx); err != nil {
		return nil, nil, err
	}
	if has, err := r.hasBlockingRiddleTx(ctx, tx); err != nil {
		return nil, nil, err
	} else if has {
		return nil, nil, ErrRiddleAlreadyActive
	}

	var rid Riddle
	err := tx.QueryRow(ctx, `
		INSERT INTO riddles (state, post_text, reward_amount, created_by_admin_id, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, state, post_text, reward_amount, group_chat_id, message_id, created_by_admin_id, created_at, published_at, finished_at, expires_at
	`, riddleStatePublishing, postText, reward, adminID, now.UTC().Add(riddleTTL)).Scan(
		&rid.ID, &rid.State, &rid.PostText, &rid.RewardAmount, &rid.GroupChatID, &rid.MessageID,
		&rid.CreatedByAdminID, &rid.CreatedAt, &rid.PublishedAt, &rid.FinishedAt, &rid.ExpiresAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("insert publishing riddle: %w", err)
	}

	resultAnswers := make([]*RiddleAnswer, 0, len(answers))
	for _, ans := range answers {
		var row RiddleAnswer
		err = tx.QueryRow(ctx, `
			INSERT INTO riddle_answers (riddle_id, answer_raw, answer_normalized)
			VALUES ($1, $2, $3)
			RETURNING id, riddle_id, answer_raw, answer_normalized, winner_user_id, winner_message_id, winner_display, won_at
		`, rid.ID, ans.Raw, ans.Normalized).Scan(
			&row.ID, &row.RiddleID, &row.AnswerRaw, &row.AnswerNormalized, &row.WinnerUserID, &row.WinnerMessageID, &row.WinnerDisplay, &row.WonAt,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("insert riddle answer: %w", err)
		}
		resultAnswers = append(resultAnswers, &row)
	}
	return &rid, resultAnswers, nil
}

func (r *RiddleRepository) ActivatePublishedRiddle(ctx context.Context, riddleID, groupChatID, messageID int64, publishedAt time.Time) error {
	cmd, err := r.db.Exec(ctx, `
		UPDATE riddles
		SET state = $2,
		    group_chat_id = $3,
		    message_id = $4,
		    published_at = $5,
		    expires_at = $6
		WHERE id = $1 AND state = $7
	`, riddleID, riddleStateActive, groupChatID, messageID, publishedAt.UTC(), publishedAt.UTC().Add(riddleTTL), riddleStatePublishing)
	if err != nil {
		return fmt.Errorf("activate published riddle: %w", err)
	}
	if cmd.RowsAffected() != 1 {
		return ErrRiddleStateConflict
	}
	return nil
}

func (r *RiddleRepository) AbortPublishingRiddle(ctx context.Context, riddleID int64) error {
	return r.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := r.lockTx(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM riddles WHERE id = $1 AND state = $2`, riddleID, riddleStatePublishing); err != nil {
			return fmt.Errorf("abort publishing riddle: %w", err)
		}
		return nil
	})
}

func (r *RiddleRepository) StopActiveRiddleTx(ctx context.Context, tx pgx.Tx, now time.Time) (*Riddle, []*RiddleAnswer, error) {
	if err := r.lockTx(ctx, tx); err != nil {
		return nil, nil, err
	}
	rdl, err := r.getActiveRiddleTx(ctx, tx, true, now)
	if err != nil || rdl == nil {
		return nil, nil, err
	}
	cmd, err := tx.Exec(ctx, `
		UPDATE riddles
		SET state = $2, finished_at = $3, expires_at = $4
		WHERE id = $1 AND state = $5
	`, rdl.ID, riddleStateStopped, now.UTC(), now.UTC().Add(riddleTTL), riddleStateActive)
	if err != nil {
		return nil, nil, fmt.Errorf("stop active riddle: %w", err)
	}
	if cmd.RowsAffected() != 1 {
		return nil, nil, ErrRiddleStateConflict
	}
	rdl.State = riddleStateStopped
	rdl.FinishedAt = ptrTime(now.UTC())
	rdl.ExpiresAt = now.UTC().Add(riddleTTL)
	answers, err := r.listAnswersTx(ctx, tx, rdl.ID)
	if err != nil {
		return nil, nil, err
	}
	return rdl, answers, nil
}

func (r *RiddleRepository) ClaimAnswerAndMaybeCompleteTx(ctx context.Context, tx pgx.Tx, normalized, winnerDisplay string, userID int64, messageID int64, now time.Time) (*Riddle, []*RiddleAnswer, bool, error) {
	if err := r.lockTx(ctx, tx); err != nil {
		return nil, nil, false, err
	}
	rdl, err := r.getActiveRiddleTx(ctx, tx, true, now)
	if err != nil || rdl == nil {
		return nil, nil, false, err
	}
	cmd, err := tx.Exec(ctx, `
		UPDATE riddle_answers
		SET winner_user_id = $2,
		    winner_message_id = $3,
		    winner_display = $4,
		    won_at = $5
		WHERE riddle_id = $1
		  AND answer_normalized = $6
		  AND winner_user_id IS NULL
	`, rdl.ID, userID, messageID, winnerDisplay, now.UTC(), normalized)
	if err != nil {
		return nil, nil, false, fmt.Errorf("claim riddle answer: %w", err)
	}
	claimed := cmd.RowsAffected() == 1
	if !claimed {
		return nil, nil, false, nil
	}

	unanswered, err := r.countUnansweredAnswersTx(ctx, tx, rdl.ID)
	if err != nil {
		return nil, nil, false, err
	}
	if unanswered > 0 {
		return rdl, nil, false, nil
	}

	cmd, err = tx.Exec(ctx, `
		UPDATE riddles
		SET state = $2,
		    finished_at = $3,
		    expires_at = $4
		WHERE id = $1 AND state = $5
	`, rdl.ID, riddleStateCompleted, now.UTC(), now.UTC().Add(riddleTTL), riddleStateActive)
	if err != nil {
		return nil, nil, false, fmt.Errorf("complete riddle: %w", err)
	}
	if cmd.RowsAffected() != 1 {
		return nil, nil, false, ErrRiddleStateConflict
	}

	rdl.State = riddleStateCompleted
	rdl.FinishedAt = ptrTime(now.UTC())
	rdl.ExpiresAt = now.UTC().Add(riddleTTL)
	answers, err := r.listAnswersTx(ctx, tx, rdl.ID)
	if err != nil {
		return nil, nil, false, err
	}
	return rdl, answers, true, nil
}

func (r *RiddleRepository) GetActiveRiddle(ctx context.Context, now time.Time) (*Riddle, error) {
	return r.getActiveRiddleTx(ctx, r.db, false, now)
}

func (r *RiddleRepository) CleanupExpired(ctx context.Context, now time.Time) (int64, error) {
	cmd, err := r.db.Exec(ctx, `DELETE FROM riddles WHERE expires_at <= $1`, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("cleanup expired riddles: %w", err)
	}
	return cmd.RowsAffected(), nil
}

func (r *RiddleRepository) ListExpiredActiveRiddles(ctx context.Context, now time.Time) ([]*Riddle, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, state, post_text, reward_amount, group_chat_id, message_id, created_by_admin_id, created_at, published_at, finished_at, expires_at
		FROM riddles
		WHERE state = $1 AND expires_at <= $2
		ORDER BY expires_at ASC, id ASC
	`, riddleStateActive, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("list expired active riddles: %w", err)
	}
	defer rows.Close()

	var out []*Riddle
	for rows.Next() {
		var rdl Riddle
		if err := rows.Scan(
			&rdl.ID, &rdl.State, &rdl.PostText, &rdl.RewardAmount, &rdl.GroupChatID, &rdl.MessageID,
			&rdl.CreatedByAdminID, &rdl.CreatedAt, &rdl.PublishedAt, &rdl.FinishedAt, &rdl.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan expired active riddle: %w", err)
		}
		out = append(out, &rdl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired active riddles: %w", err)
	}
	return out, nil
}

func (r *RiddleRepository) hasBlockingRiddleTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM riddles WHERE state IN ($1, $2)
		)
	`, riddleStatePublishing, riddleStateActive).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check blocking riddle: %w", err)
	}
	return exists, nil
}

func (r *RiddleRepository) getActiveRiddleTx(ctx context.Context, q riddleQuerier, forUpdate bool, now time.Time) (*Riddle, error) {
	query := `
		SELECT id, state, post_text, reward_amount, group_chat_id, message_id, created_by_admin_id, created_at, published_at, finished_at, expires_at
		FROM riddles
		WHERE state = $1 AND expires_at > $2
		ORDER BY published_at DESC NULLS LAST, created_at DESC
		LIMIT 1`
	if forUpdate {
		query += ` FOR UPDATE`
	}

	var rdl Riddle
	err := q.QueryRow(ctx, query, riddleStateActive, now.UTC()).Scan(
		&rdl.ID, &rdl.State, &rdl.PostText, &rdl.RewardAmount, &rdl.GroupChatID, &rdl.MessageID,
		&rdl.CreatedByAdminID, &rdl.CreatedAt, &rdl.PublishedAt, &rdl.FinishedAt, &rdl.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get active riddle: %w", err)
	}
	return &rdl, nil
}

func (r *RiddleRepository) listAnswersTx(ctx context.Context, q riddleQuerier, riddleID int64) ([]*RiddleAnswer, error) {
	rows, err := q.Query(ctx, `
		SELECT id, riddle_id, answer_raw, answer_normalized, winner_user_id, winner_message_id, winner_display, won_at
		FROM riddle_answers
		WHERE riddle_id = $1
		ORDER BY id ASC
	`, riddleID)
	if err != nil {
		return nil, fmt.Errorf("list riddle answers: %w", err)
	}
	defer rows.Close()

	var out []*RiddleAnswer
	for rows.Next() {
		var ans RiddleAnswer
		if err := rows.Scan(&ans.ID, &ans.RiddleID, &ans.AnswerRaw, &ans.AnswerNormalized, &ans.WinnerUserID, &ans.WinnerMessageID, &ans.WinnerDisplay, &ans.WonAt); err != nil {
			return nil, fmt.Errorf("scan riddle answer: %w", err)
		}
		out = append(out, &ans)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate riddle answers: %w", err)
	}
	return out, nil
}

func (r *RiddleRepository) countUnansweredAnswersTx(ctx context.Context, q riddleQuerier, riddleID int64) (int64, error) {
	var unanswered int64
	err := q.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM riddle_answers
		WHERE riddle_id = $1
		  AND winner_user_id IS NULL
	`, riddleID).Scan(&unanswered)
	if err != nil {
		return 0, fmt.Errorf("count unanswered riddle answers: %w", err)
	}
	return unanswered, nil
}

func ptrTime(v time.Time) *time.Time {
	return &v
}

func rollbackRiddleOnFailure(ctx context.Context, tx pgx.Tx, errp *error) {
	if errp == nil || *errp == nil {
		return
	}
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		*errp = errors.Join(*errp, fmt.Errorf("rollback riddle tx: %w", rollbackErr))
	}
}
