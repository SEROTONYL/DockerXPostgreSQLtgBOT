// Package common — errors.go определяет пользовательские ошибки,
// которые используются во всех модулях бота.
// Эти ошибки позволяют обработчикам различать типы проблем
// и отправлять пользователю понятные сообщения.
package common

import "errors"

// Ошибки экономики (пленки, переводы)
var (
	// ErrInsufficientBalance — недостаточно пленок на счёте
	ErrInsufficientBalance = errors.New("недостаточно 🎞️ на счёте")
	// ErrSelfTransfer — попытка перевести пленки самому себе
	ErrSelfTransfer = errors.New("нельзя переводить 🎞️ самому себе")
	// ErrInvalidAmount — некорректная сумма (ноль или отрицательная)
	ErrInvalidAmount = errors.New("сумма должна быть положительной")
	// ErrUserNotFound — пользователь не найден в базе
	ErrUserNotFound = errors.New("пользователь не найден")
	// ErrTransferTargetIsBot — нельзя использовать бота как получателя перевода
	ErrTransferTargetIsBot = errors.New("нельзя переводить 🎞️ ботам")
)

// Ошибки кармы
var (
	// ErrKarmaDailyLimit — лимит кармы на день исчерпан
	ErrKarmaDailyLimit = errors.New("лимит кармы на сегодня исчерпан (2 в день)")
	// ErrKarmaSelfGive — попытка дать карму самому себе
	ErrKarmaSelfGive = errors.New("нельзя давать карму самому себе")
	// ErrKarmaAlreadyGave — уже давали карму этому пользователю сегодня
	ErrKarmaAlreadyGave = errors.New("вы уже давали карму этому пользователю сегодня")
)

var (
	ErrThanksTargetMissing      = errors.New("не указан получатель благодарности")
	ErrThanksMalformedCommand   = errors.New("некорректная команда благодарности")
	ErrThanksSelfGive           = errors.New("нельзя благодарить самого себя")
	ErrThanksTargetIsBot        = errors.New("нельзя благодарить бота")
	ErrThanksDailyLimit         = errors.New("дневной лимит благодарностей исчерпан")
	ErrThanksReciprocalCooldown = errors.New("ответная благодарность временно недоступна")
)

// Ошибки админки
var (
	// ErrNotAdmin — пользователь не является администратором
	ErrNotAdmin = errors.New("у вас нет прав администратора")
	// ErrWrongPassword — неверный пароль
	ErrWrongPassword = errors.New("неверный пароль")
	// ErrTooManyAttempts — слишком много неудачных попыток входа
	ErrTooManyAttempts = errors.New("слишком много попыток, подождите 1 час")
	// ErrSessionExpired — сессия истекла
	ErrSessionExpired = errors.New("сессия истекла, авторизуйтесь заново")
	// ErrRoleTooLong — роль длиннее 64 символов
	ErrRoleTooLong = errors.New("роль слишком длинная (максимум 64 символа)")
)

// Ошибки казино
var (
	// ErrCasinoDisabled — казино отключено в настройках
	ErrCasinoDisabled = errors.New("казино временно отключено")
)
