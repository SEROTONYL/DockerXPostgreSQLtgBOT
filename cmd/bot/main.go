// Package main — точка входа бота.
// Загружает конфигурацию, инициализирует приложение и запускает.
// Поддерживает graceful shutdown по SIGINT/SIGTERM.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/app"
	"telegram-bot/internal/config"
)

func main() {
	// Настраиваем логирование
	setupLogging()

	log.Info("=== Бот запускается ===")

	// Загружаем конфигурацию из переменных окружения
	cfg, err := config.Load()
	if err != nil {
		log.WithError(err).Fatal("Не удалось загрузить конфигурацию")
	}

	// Устанавливаем уровень логирования из конфига
	level, err := log.ParseLevel(cfg.AppLogLevel)
	if err == nil {
		log.SetLevel(level)
	}

	// Контекст с отменой для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализируем приложение (БД, бот, сервисы, обработчики)
	application, err := app.New(ctx, cfg)
	if err != nil {
		log.WithError(err).Fatal("Не удалось инициализировать приложение")
	}
	defer application.DB.Close()

	// Запускаем планировщик задач (cron)
	application.Scheduler.Start(ctx)
	defer application.Scheduler.Stop()

	// Обрабатываем сигналы остановки (Ctrl+C, docker stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Запускаем бота в отдельной горутине
	go application.Bot.Start(ctx)

	log.Info("=== Бот готов к работе ===")

	// Ждём сигнала остановки
	sig := <-quit
	log.Infof("Получен сигнал %s, останавливаемся...", sig)

	// Отменяем контекст — все горутины начнут завершаться
	cancel()

	log.Info("=== Бот остановлен ===")
}

// setupLogging настраивает формат логов.
func setupLogging() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
}
