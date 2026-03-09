package admin

import (
	"time"

	"serotonyl.ru/telegram-bot/internal/uiwizard"
)

const (
	riddleStatePublishing = "publishing"
	riddleStateActive     = "active"
	riddleStateCompleted  = "completed"
	riddleStateStopped    = "stopped"

	riddleTTL = 24 * time.Hour
)

type Riddle struct {
	ID               int64      `db:"id"`
	State            string     `db:"state"`
	PostText         string     `db:"post_text"`
	RewardAmount     int64      `db:"reward_amount"`
	GroupChatID      *int64     `db:"group_chat_id"`
	MessageID        *int64     `db:"message_id"`
	CreatedByAdminID int64      `db:"created_by_admin_id"`
	CreatedAt        time.Time  `db:"created_at"`
	PublishedAt      *time.Time `db:"published_at"`
	FinishedAt       *time.Time `db:"finished_at"`
	ExpiresAt        time.Time  `db:"expires_at"`
}

type RiddleAnswer struct {
	ID               int64      `db:"id"`
	RiddleID         int64      `db:"riddle_id"`
	AnswerRaw        string     `db:"answer_raw"`
	AnswerNormalized string     `db:"answer_normalized"`
	WinnerUserID     *int64     `db:"winner_user_id"`
	WinnerMessageID  *int64     `db:"winner_message_id"`
	WinnerDisplay    *string    `db:"winner_display"`
	WonAt            *time.Time `db:"won_at"`
}

type RiddleDraftAnswer struct {
	Raw        string `json:"raw"`
	Normalized string `json:"normalized"`
}

type RiddleDraftData struct {
	Wizard       *uiwizard.WizardState `json:"wizard,omitempty"`
	PostText     string                `json:"post_text"`
	Answers      []RiddleDraftAnswer   `json:"answers"`
	RewardAmount int64                 `json:"reward_amount"`
}

type RiddlePublishResult struct {
	Riddle   *Riddle
	Answers  []*RiddleAnswer
	PostedTo int64
}

type RiddleStopResult struct {
	Riddle  *Riddle
	Answers []*RiddleAnswer
}

type RiddleCompletionResult struct {
	Riddle  *Riddle
	Answers []*RiddleAnswer
}
