// Package middleware — recovery.go защищает от паник.
// Если обработчик паникует — ловим, логируем и продолжаем работу.
package middleware

import (
	"fmt"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
)

// RecoverFromPanic восстанавливает горутину после паники.
// Вызывается через defer RecoverFromPanic() в начале обработчика.
func RecoverFromPanic() {
	if r := recover(); r != nil {
		log.WithFields(log.Fields{
			"panic": fmt.Sprintf("%v", r),
			"stack": string(debug.Stack()),
		}).Error("ПАНИКА в обработчике — восстановлено")
	}
}
