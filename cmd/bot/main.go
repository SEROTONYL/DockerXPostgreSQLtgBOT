// Package main вЂ” С‚РѕС‡РєР° РІС…РѕРґР° Р±РѕС‚Р°.
// Р—Р°РіСЂСѓР¶Р°РµС‚ РєРѕРЅС„РёРіСѓСЂР°С†РёСЋ, РёРЅРёС†РёР°Р»РёР·РёСЂСѓРµС‚ РїСЂРёР»РѕР¶РµРЅРёРµ Рё Р·Р°РїСѓСЃРєР°РµС‚.
// РџРѕРґРґРµСЂР¶РёРІР°РµС‚ graceful shutdown РїРѕ SIGINT/SIGTERM.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/app"
	"serotonyl.ru/telegram-bot/internal/config"
)

func main() {
	// РќР°СЃС‚СЂР°РёРІР°РµРј Р»РѕРіРёСЂРѕРІР°РЅРёРµ
	setupLogging()

	log.Info("=== Р‘РѕС‚ Р·Р°РїСѓСЃРєР°РµС‚СЃСЏ ===")

	// Р—Р°РіСЂСѓР¶Р°РµРј РєРѕРЅС„РёРіСѓСЂР°С†РёСЋ РёР· РїРµСЂРµРјРµРЅРЅС‹С… РѕРєСЂСѓР¶РµРЅРёСЏ
	cfg, err := config.Load()
	if err != nil {
		log.WithError(err).Fatal("РќРµ СѓРґР°Р»РѕСЃСЊ Р·Р°РіСЂСѓР·РёС‚СЊ РєРѕРЅС„РёРіСѓСЂР°С†РёСЋ")
	}

	// РЈСЃС‚Р°РЅР°РІР»РёРІР°РµРј СѓСЂРѕРІРµРЅСЊ Р»РѕРіРёСЂРѕРІР°РЅРёСЏ РёР· РєРѕРЅС„РёРіР°
	level, err := log.ParseLevel(cfg.AppLogLevel)
	if err == nil {
		log.SetLevel(level)
	}

	// РљРѕРЅС‚РµРєСЃС‚ СЃ РѕС‚РјРµРЅРѕР№ РґР»СЏ graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// РРЅРёС†РёР°Р»РёР·РёСЂСѓРµРј РїСЂРёР»РѕР¶РµРЅРёРµ (Р‘Р”, Р±РѕС‚, СЃРµСЂРІРёСЃС‹, РѕР±СЂР°Р±РѕС‚С‡РёРєРё)
	application, err := app.New(ctx, cfg)
	if err != nil {
		log.WithError(err).Fatal("РќРµ СѓРґР°Р»РѕСЃСЊ РёРЅРёС†РёР°Р»РёР·РёСЂРѕРІР°С‚СЊ РїСЂРёР»РѕР¶РµРЅРёРµ")
	}
	defer application.DB.Close()

	// Р—Р°РїСѓСЃРєР°РµРј РїР»Р°РЅРёСЂРѕРІС‰РёРє Р·Р°РґР°С‡ (cron)
	application.Scheduler.Start(ctx)
	defer application.Scheduler.Stop()

	// РћР±СЂР°Р±Р°С‚С‹РІР°РµРј СЃРёРіРЅР°Р»С‹ РѕСЃС‚Р°РЅРѕРІРєРё (Ctrl+C, docker stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Р—Р°РїСѓСЃРєР°РµРј Р±РѕС‚Р° РІ РѕС‚РґРµР»СЊРЅРѕР№ РіРѕСЂСѓС‚РёРЅРµ
	go application.Bot.Start(ctx)

	log.Info("=== Р‘РѕС‚ РіРѕС‚РѕРІ Рє СЂР°Р±РѕС‚Рµ ===")

	// Р–РґС‘Рј СЃРёРіРЅР°Р»Р° РѕСЃС‚Р°РЅРѕРІРєРё
	sig := <-quit
	log.Infof("РџРѕР»СѓС‡РµРЅ СЃРёРіРЅР°Р» %s, РѕСЃС‚Р°РЅР°РІР»РёРІР°РµРјСЃСЏ...", sig)

	// РћС‚РјРµРЅСЏРµРј РєРѕРЅС‚РµРєСЃС‚ вЂ” РІСЃРµ РіРѕСЂСѓС‚РёРЅС‹ РЅР°С‡РЅСѓС‚ Р·Р°РІРµСЂС€Р°С‚СЊСЃСЏ
	cancel()

	log.Info("=== Р‘РѕС‚ РѕСЃС‚Р°РЅРѕРІР»РµРЅ ===")
}

// setupLogging РЅР°СЃС‚СЂР°РёРІР°РµС‚ С„РѕСЂРјР°С‚ Р»РѕРіРѕРІ.
func setupLogging() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
}
