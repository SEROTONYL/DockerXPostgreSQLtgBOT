package feature

import "serotonyl.ru/telegram-bot/internal/commands"

// Feature описывает единый интерфейс подключения фич к командному роутеру.
type Feature interface {
	Name() string
	RegisterCommands(r *commands.Router)
}
