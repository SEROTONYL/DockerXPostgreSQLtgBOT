package bot

import (
	"context"
	"errors"
	"strings"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/commands"
)

var errAdminCommandNotAllowed = errors.New("admin chat command not allowed")

// routeCommand маршрутизирует команду к нужному обработчику.
func isAdminChatAllowedCommand(cmd string) bool {
	switch cmd {
	case "members_status", "members_stats":
		return true
	default:
		return false
	}
}

func (b *Bot) canExecuteCommand(uc UpdateContext, cmd string) error {
	if uc.IsAdminChat && !isAdminChatAllowedCommand(cmd) {
		return errAdminCommandNotAllowed
	}
	return nil
}

func (b *Bot) routeCommand(ctx context.Context, uc UpdateContext, cmd string, args []string) {
	log.WithFields(log.Fields{
		"cmd":  cmd,
		"args": args,
	}).Debug("routing command")

	if err := b.canExecuteCommand(uc, cmd); err != nil {
		return
	}

	c := commands.Context{
		ChatID:      uc.ChatID,
		UserID:      uc.UserID,
		IsPrivate:   uc.IsPrivate,
		IsAdminChat: uc.IsAdminChat,
		Now:         uc.Now,
	}

	if ok := b.cmdRouter.Dispatch(ctx, c, cmd, args); !ok {
		log.WithField("cmd", cmd).Debug("unknown command")
	}
}

// CommandParser парсит русские команды с префиксами ! и .
type CommandParser struct {
	validPrefixes []string
}

// NewCommandParser создаёт парсер команд.
func NewCommandParser() *CommandParser {
	return &CommandParser{validPrefixes: []string{"!", ".", "/"}}
}

// ParseCommand разбирает текст на команду и аргументы.
func (p *CommandParser) ParseCommand(text string, allowSlash bool) (string, []string, bool) {
	text = strings.TrimSpace(text)

	hasPrefix := false
	for _, prefix := range p.validPrefixes {
		if prefix == "/" && !allowSlash {
			continue
		}
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimPrefix(text, prefix)
			hasPrefix = true
			break
		}
	}

	if !hasPrefix {
		return "", nil, false
	}

	text = strings.TrimSpace(text)
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", nil, false
	}

	command := strings.ToLower(parts[0])
	command = strings.ReplaceAll(command, "ё", "е")
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	return command, args, true
}
