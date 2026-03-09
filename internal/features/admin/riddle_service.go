package admin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	models "github.com/mymmrac/telego"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

var (
	ErrRiddleAlreadyActive = errors.New("riddle already active")
	ErrRiddleNotFound      = errors.New("riddle not found")
	ErrRiddleStateConflict = errors.New("riddle state conflict")
)

type riddleEconomy interface {
	WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
	AddBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error
}

type riddleRepo interface {
	WithTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
	CreatePublishingRiddleTx(ctx context.Context, tx pgx.Tx, adminID int64, postText string, reward int64, answers []RiddleDraftAnswer, now time.Time) (*Riddle, []*RiddleAnswer, error)
	ActivatePublishedRiddle(ctx context.Context, riddleID, groupChatID, messageID int64, publishedAt time.Time) error
	AbortPublishingRiddle(ctx context.Context, riddleID int64) error
	StopActiveRiddleTx(ctx context.Context, tx pgx.Tx, now time.Time) (*Riddle, []*RiddleAnswer, error)
	ClaimAnswerAndMaybeCompleteTx(ctx context.Context, tx pgx.Tx, normalized, winnerDisplay string, userID int64, messageID int64, now time.Time) (*Riddle, []*RiddleAnswer, bool, error)
	GetActiveRiddle(ctx context.Context, now time.Time) (*Riddle, error)
	ListExpiredActiveRiddles(ctx context.Context, now time.Time) ([]*Riddle, error)
	CleanupExpired(ctx context.Context, now time.Time) (int64, error)
}

type RiddleService struct {
	repo    riddleRepo
	economy riddleEconomy
	ops     *telegram.Ops
	now     func() time.Time
}

func NewRiddleService(repo riddleRepo, economy riddleEconomy) *RiddleService {
	return &RiddleService{
		repo:    repo,
		economy: economy,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *RiddleService) CreatePublishing(ctx context.Context, adminID int64, draft *RiddleDraftData) (*RiddlePublishResult, error) {
	if draft == nil {
		return nil, fmt.Errorf("riddle draft is nil")
	}
	now := s.now()
	var (
		rdl     *Riddle
		answers []*RiddleAnswer
	)
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rdl, answers, err = s.repo.CreatePublishingRiddleTx(ctx, tx, adminID, draft.PostText, draft.RewardAmount, draft.Answers, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &RiddlePublishResult{Riddle: rdl, Answers: answers}, nil
}

func (s *RiddleService) ActivatePublished(ctx context.Context, riddleID, groupChatID, messageID int64) error {
	return s.repo.ActivatePublishedRiddle(ctx, riddleID, groupChatID, messageID, s.now())
}

func (s *RiddleService) AbortPublication(ctx context.Context, riddleID int64) error {
	return s.repo.AbortPublishingRiddle(ctx, riddleID)
}

func (s *RiddleService) SetOps(ops *telegram.Ops) {
	s.ops = ops
}

func (s *RiddleService) GetActive(ctx context.Context) (*Riddle, error) {
	return s.repo.GetActiveRiddle(ctx, s.now())
}

func (s *RiddleService) StopActive(ctx context.Context) (*RiddleStopResult, error) {
	if s.economy == nil {
		return nil, fmt.Errorf("riddle economy is nil")
	}
	now := s.now()
	var (
		rdl     *Riddle
		answers []*RiddleAnswer
	)
	err := s.economy.WithTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rdl, answers, err = s.repo.StopActiveRiddleTx(ctx, tx, now)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrRiddleNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if rdl == nil {
		return nil, nil
	}
	return &RiddleStopResult{Riddle: rdl, Answers: answers}, nil
}

func (s *RiddleService) ProcessGuess(ctx context.Context, message *models.Message) (*RiddleCompletionResult, bool, error) {
	if s.economy == nil {
		return nil, false, fmt.Errorf("riddle economy is nil")
	}
	if message == nil || message.From == nil {
		return nil, false, nil
	}
	normalized := normalizeRiddleText(message.Text)
	if normalized == "" {
		return nil, false, nil
	}
	winnerDisplay := visibleRiddleUserName(*message.From)
	now := s.now()
	var (
		rdl       *Riddle
		answers   []*RiddleAnswer
		completed bool
	)
	err := s.economy.WithTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rdl, answers, completed, err = s.repo.ClaimAnswerAndMaybeCompleteTx(ctx, tx, normalized, winnerDisplay, message.From.ID, int64(message.MessageID), now)
		if err != nil || !completed {
			return err
		}
		for _, ans := range answers {
			if ans.WinnerUserID == nil {
				continue
			}
			if err := s.economy.AddBalanceTx(ctx, tx, *ans.WinnerUserID, rdl.RewardAmount, "riddle_reward", fmt.Sprintf("Riddle %d reward", rdl.ID)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrRiddleNotFound) || errors.Is(err, ErrRiddleStateConflict) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if rdl == nil {
		return nil, false, nil
	}
	if !completed {
		return nil, true, nil
	}
	return &RiddleCompletionResult{Riddle: rdl, Answers: answers}, true, nil
}

func (s *RiddleService) CleanupExpired(ctx context.Context, now time.Time) error {
	expired, err := s.repo.ListExpiredActiveRiddles(ctx, now)
	if err != nil {
		return err
	}
	if s.ops != nil {
		for _, rdl := range expired {
			if rdl == nil || rdl.GroupChatID == nil || rdl.MessageID == nil {
				continue
			}
			_ = s.ops.UnpinChatMessage(ctx, *rdl.GroupChatID, int(*rdl.MessageID))
		}
	}
	_, err = s.repo.CleanupExpired(ctx, now)
	return err
}

func normalizeRiddleText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func visibleRiddleUserName(user models.User) string {
	username := strings.TrimPrefix(strings.TrimSpace(user.Username), "@")
	if username != "" {
		return "@" + username
	}
	name := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(user.FirstName), strings.TrimSpace(user.LastName)}, " "))
	if name != "" {
		return name
	}
	return fmt.Sprintf("id:%d", user.ID)
}

func summarizeRiddleWinners(answers []*RiddleAnswer) []string {
	counts := map[string]int{}
	order := make([]string, 0)
	for _, ans := range answers {
		if ans == nil || ans.WinnerUserID == nil || ans.WinnerDisplay == nil || strings.TrimSpace(*ans.WinnerDisplay) == "" {
			continue
		}
		key := strings.TrimSpace(*ans.WinnerDisplay)
		if _, ok := counts[key]; !ok {
			order = append(order, key)
		}
		counts[key]++
	}
	sort.SliceStable(order, func(i, j int) bool { return i < j })
	out := make([]string, 0, len(order))
	for _, key := range order {
		if counts[key] > 1 {
			out = append(out, fmt.Sprintf("%s x%d", key, counts[key]))
			continue
		}
		out = append(out, key)
	}
	return out
}
