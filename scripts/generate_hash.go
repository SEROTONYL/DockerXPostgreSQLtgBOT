// +build ignore

// generate_hash.go — утилита для генерации Argon2id хеша пароля.
// Запуск: go run scripts/generate_hash.go ваш_пароль
//
// Результат вставьте в .env как ADMIN_PASSWORD_HASH.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: go run scripts/generate_hash.go <пароль>")
		os.Exit(1)
	}

	password := os.Args[1]

	// Генерируем случайную соль (16 байт)
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		fmt.Printf("Ошибка генерации соли: %v\n", err)
		os.Exit(1)
	}

	// Параметры Argon2id
	var (
		memory      uint32 = 65536 // 64 MB
		iterations  uint32 = 3
		parallelism uint8  = 2
		keyLength   uint32 = 32
	)

	// Вычисляем хеш
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	// Форматируем в стандартный формат
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	result := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, iterations, parallelism, encodedSalt, encodedHash)

	fmt.Println("Хеш пароля (вставьте в .env как ADMIN_PASSWORD_HASH):")
	fmt.Println(result)
}
