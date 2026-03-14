// Package admin реализует админ-панель с парольной аутентификацией.
// models.go описывает структуры сессий и попыток входа.
package admin

import (
	"time"

	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/uiwizard"
)

// AdminSession — активная сессия администратора.
type AdminSession struct {
	ID              int64     `db:"id"`
	UserID          int64     `db:"user_id"`
	SessionToken    string    `db:"session_token"`
	AuthenticatedAt time.Time `db:"authenticated_at"`
	ExpiresAt       time.Time `db:"expires_at"`
	LastActivity    time.Time `db:"last_activity"`
	IsActive        bool      `db:"is_active"`
}

// LoginAttempt — попытка входа (для защиты от brute-force).
type LoginAttempt struct {
	ID          int64     `db:"id"`
	UserID      int64     `db:"user_id"`
	AttemptTime time.Time `db:"attempt_time"`
	Success     bool      `db:"success"`
}

// AdminState — состояние диалога с админом (конечный автомат).
// Админ-панель работает по шагам: выбор действия → выбор пользователя → ввод роли.
type AdminState struct {
	State      string      // Текущее состояние ("", "awaiting_password", "assign_role_select", ...)
	Data       interface{} // Данные контекста (список пользователей, выбранный пользователь)
	PanelMsgID int         // message_id «панельного» сообщения для editMessage* single-thread UI
	ExpiresAt  time.Time   // Когда состояние истекает (5 минут)
}

// UserPickerMode определяет режим выборщика пользователей.
type AdminPanelMessage struct {
	ChatID    int64
	MessageID int
}

type UserPickerMode string

const (
	UserPickerAssignWithoutRole UserPickerMode = "assign_without_role"
	UserPickerChangeWithRole    UserPickerMode = "change_with_role"
)

// UserPickerData хранит состояние постраничного выбора пользователя в админ-флоу.
type UserPickerData struct {
	Mode           UserPickerMode
	UsersSnapshot  []*members.Member
	PageIndex      int
	PageSize       int
	SelectedUserID int64
}

// RoleInputData хранит контекст шага ввода роли и возврата «Назад» к выбору участника.
type RoleInputData struct {
	SelectedUser *members.Member
	Picker       *UserPickerData
}

type BalanceAdjustMode string

const (
	BalanceAdjustModeAdd    BalanceAdjustMode = "add"
	BalanceAdjustModeDeduct BalanceAdjustMode = "deduct"
)

type BalanceAdjustOperation struct {
	UserID int64
	Mode   BalanceAdjustMode
	Amount int64
}

type BalanceAdjustData struct {
	Wizard           *uiwizard.WizardState
	Mode             BalanceAdjustMode
	ReturnScreen     string
	FlowChatID       int64
	FlowMessageID    int
	FlowStartedAt    time.Time
	UsersSnapshot    []*members.Member
	SelectedUserIDs  map[int64]bool
	PageIndex        int
	PageSize         int
	Amount           int64
	AmountSource     string
	AwaitingManual   bool
	PendingDeltaName string
	LastOperation    []BalanceAdjustOperation
	LastOperationID  string
	LastOperationAt  time.Time
	Undone           bool
}

type BalanceDelta struct {
	ID        int64
	ChatID    int64
	Name      string
	Amount    int64
	CreatedBy int64
	CreatedAt time.Time
}

// Возможные состояния админ-диалога
const (
	StateNone                 = ""                   // Нет активного состояния
	StateAwaitingPassword     = "awaiting_password"  // Ждём пароль
	StateAssignRoleSelect     = "assign_role_select" // Ждём выбор пользователя (без роли)
	StateAssignRoleText       = "assign_role_text"   // Ждём текст новой роли
	StateChangeRoleSelect     = "change_role_select" // Ждём выбор пользователя (с ролью)
	StateChangeRoleText       = "change_role_text"   // Ждём новую роль
	StateBalanceAdjustMode    = "admin:balance_adjust_mode"
	StateBalanceAdjustPicker  = "admin:balance_adjust_picker"
	StateBalanceAdjustAmount  = "admin:balance_adjust_amount"
	StateBalanceAdjustConfirm = "admin:balance_adjust_confirm"
	StateBalanceDeltaName     = "admin:balance_delta_name"
	StateBalanceDeltaAmount   = "admin:balance_delta_amount"
	StateRiddleText           = "admin:riddle_text"
	StateRiddleAnswers        = "admin:riddle_answers"
	StateRiddleReward         = "admin:riddle_reward"
	StateRiddleConfirm        = "admin:riddle_confirm"
)
