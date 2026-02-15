// Package casino — rtp.go реализует динамическую систему RTP (Return To Player).
// RTP корректирует веса символов для каждого пользователя:
//   - Если пользователь выигрывает слишком много (RTP > 98%) — уменьшаем шансы
//   - Если проигрывает слишком много (RTP < 94%) — увеличиваем шансы
//
// Целевой диапазон RTP: 94–98%.
package casino

import (
	"sync"
)

// RTPManager управляет динамическими весами символов для каждого пользователя.
// Использует мьютекс для потокобезопасности, т.к. несколько спинов
// могут происходить одновременно.
type RTPManager struct {
	mu            sync.RWMutex
	userWeights   map[int64][]Symbol // Персональные веса символов
	minRTP        float64            // Минимальный целевой RTP (94%)
	maxRTP        float64            // Максимальный целевой RTP (98%)
	initialRTP    float64            // Начальный RTP (96%)
}

// NewRTPManager создаёт менеджер RTP с заданными границами.
func NewRTPManager(minRTP, maxRTP, initialRTP float64) *RTPManager {
	return &RTPManager{
		userWeights: make(map[int64][]Symbol),
		minRTP:      minRTP,
		maxRTP:      maxRTP,
		initialRTP:  initialRTP,
	}
}

// GetAdjustedWeights возвращает скорректированные веса символов для пользователя.
// Если для пользователя нет персональных весов — возвращает стандартные.
func (m *RTPManager) GetAdjustedWeights(userID int64) []Symbol {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if weights, ok := m.userWeights[userID]; ok {
		return weights
	}

	// Копируем стандартные веса
	return copySymbols(DefaultSymbols)
}

// AdjustRTP корректирует веса символов на основе текущего RTP пользователя.
// Вызывается после каждого спина.
//
// Алгоритм:
//   - RTP > 98% → уменьшаем вес дорогих символов (💎, 7️⃣, ⭐)
//   - RTP < 94% → увеличиваем вес дорогих символов и вайлдов
//   - 94% ≤ RTP ≤ 98% → веса в норме, не трогаем
func (m *RTPManager) AdjustRTP(userID int64, currentRTP float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Начинаем со стандартных весов
	symbols := copySymbols(DefaultSymbols)

	if currentRTP > m.maxRTP {
		// Пользователь выигрывает слишком много — уменьшаем шансы
		// Уменьшаем вес дорогих символов
		for i := range symbols {
			switch symbols[i].Name {
			case "Seven":
				symbols[i].Weight = max(1, symbols[i].Weight-1) // 3 → 2
			case "Diamond":
				symbols[i].Weight = max(3, symbols[i].Weight-2) // 7 → 5
			case "Wild":
				symbols[i].Weight = max(1, symbols[i].Weight-1) // 1 → 1 (минимум)
			case "Cherry":
				symbols[i].Weight += 3 // Увеличиваем дешёвые
			case "Lemon":
				symbols[i].Weight += 2
			}
		}
	} else if currentRTP < m.minRTP {
		// Пользователь проигрывает слишком много — помогаем
		for i := range symbols {
			switch symbols[i].Name {
			case "Seven":
				symbols[i].Weight += 1 // 3 → 4
			case "Diamond":
				symbols[i].Weight += 2 // 7 → 9
			case "Wild":
				symbols[i].Weight += 1 // 1 → 2
			case "Watermelon":
				symbols[i].Weight += 2 // 10 → 12
			case "Cherry":
				symbols[i].Weight = max(15, symbols[i].Weight-5) // 25 → 20
			}
		}
	}

	m.userWeights[userID] = symbols
}

// CalculateRTP вычисляет текущий RTP пользователя.
// RTP = (Всего выиграно / Всего поставлено) × 100%
//
// Если пользователь ещё не играл — возвращает начальный RTP (96%).
func CalculateRTP(totalWagered, totalWon int64) float64 {
	if totalWagered == 0 {
		return 96.0 // По умолчанию
	}
	return (float64(totalWon) / float64(totalWagered)) * 100
}

// copySymbols создаёт глубокую копию массива символов.
// Нужно, чтобы изменения персональных весов не затронули оригинал.
func copySymbols(src []Symbol) []Symbol {
	dst := make([]Symbol, len(src))
	copy(dst, src)
	return dst
}

// max возвращает максимум из двух int.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
