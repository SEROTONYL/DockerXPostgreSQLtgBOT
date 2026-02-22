package middleware

import (
	"fmt"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
)

func RecoverFromPanic() {
	if r := recover(); r != nil {
		log.WithFields(log.Fields{
			"component": "panic_recovery",
			"panic":     fmt.Sprintf("%v", r),
			"stack":     string(debug.Stack()),
		}).Error("ПАНИКА в обработчике — восстановлено")
	}
}