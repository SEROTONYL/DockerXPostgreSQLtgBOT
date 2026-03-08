package commands

import (
	"context"
	"strings"
	"time"
)

// Context содержит данные апдейта, необходимые для обработки команд.
type Context struct {
	ChatID      int64
	UserID      int64
	IsPrivate   bool
	IsAdminChat bool
	Now         time.Time
}

// HandlerFunc — сигнатура обработчика команд.
type HandlerFunc func(ctx context.Context, c Context, args []string)

// Router хранит зарегистрированные обработчики команд.
type Router struct {
	handlers map[string]HandlerFunc
}

// NewRouter создаёт пустой роутер.
func NewRouter() *Router {
	return &Router{handlers: make(map[string]HandlerFunc)}
}

// Register регистрирует обработчик команды.
func (r *Router) Register(cmd string, h HandlerFunc) {
	norm := normalize(cmd)
	if norm == "" {
		panic("commands: empty command")
	}
	if h == nil {
		panic("commands: nil handler")
	}
	if _, exists := r.handlers[norm]; exists {
		panic("commands: duplicate command registration: " + norm)
	}
	r.handlers[norm] = h
}

// Dispatch запускает обработчик команды, если она зарегистрирована.
func (r *Router) Dispatch(ctx context.Context, c Context, cmd string, args []string) bool {
	h, ok := r.handlers[normalize(cmd)]
	if !ok {
		return false
	}
	h(ctx, c, args)
	return true
}

func normalize(cmd string) string {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	return strings.ReplaceAll(cmd, "ё", "е")
}
