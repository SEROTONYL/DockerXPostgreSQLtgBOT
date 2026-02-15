// Package casino реализует слот-машину 5x6 с вайлдами, скаттерами и динамическим RTP.
// models.go описывает все структуры данных казино.
package casino

import (
	"encoding/json"
	"time"
)

// Symbol представляет символ слот-машины.
type Symbol struct {
	Emoji  string // Эмодзи символа (🍒, 💎, 7️⃣ и т.д.)
	Name   string // Название для логов
	Weight int    // Вес (вероятность появления)
	Value  int    // Множитель выплаты
}

// DefaultSymbols — символы с начальными весами.
// Веса определяют вероятность: чем больше вес, тем чаще выпадает.
var DefaultSymbols = []Symbol{
	{Emoji: "🍒", Name: "Cherry", Weight: 25, Value: 1},     // 25% — самый частый
	{Emoji: "🍋", Name: "Lemon", Weight: 20, Value: 1},      // 20%
	{Emoji: "🍊", Name: "Orange", Weight: 18, Value: 1},     // 18%
	{Emoji: "🍇", Name: "Grape", Weight: 15, Value: 2},      // 15%
	{Emoji: "🍉", Name: "Watermelon", Weight: 10, Value: 3}, // 10%
	{Emoji: "💎", Name: "Diamond", Weight: 7, Value: 5},     // 7% — редкий
	{Emoji: "7️⃣", Name: "Seven", Weight: 3, Value: 10},      // 3% — самый редкий
	{Emoji: "⭐", Name: "Wild", Weight: 1, Value: 0},         // 1% — замена любого
	{Emoji: "🎰", Name: "Scatter", Weight: 1, Value: 0},     // 1% — бонус
}

// Константы символов
const (
	WildEmoji    = "⭐"
	ScatterEmoji = "🎰"
)

// Grid — сетка слотов 5 рилов × 6 строк.
// Grid[reel][row] — символ на конкретной позиции.
type Grid [5][6]string

// Game — запись одной игры в БД.
type Game struct {
	ID           int64           `db:"id"`
	UserID       int64           `db:"user_id"`
	GameType     string          `db:"game_type"`
	BetAmount    int64           `db:"bet_amount"`
	ResultAmount int64           `db:"result_amount"`
	GameData     json.RawMessage `db:"game_data"`
	RTPPercent   float64         `db:"rtp_percentage"`
	CreatedAt    time.Time       `db:"created_at"`
}

// Stats — статистика казино пользователя.
type Stats struct {
	ID           int64     `db:"id"`
	UserID       int64     `db:"user_id"`
	TotalSpins   int       `db:"total_spins"`
	TotalWagered int64     `db:"total_wagered"`
	TotalWon     int64     `db:"total_won"`
	BiggestWin   int64     `db:"biggest_win"`
	CurrentRTP   float64   `db:"current_rtp"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// SlotResult — результат одного спина.
type SlotResult struct {
	Grid         Grid     // Сетка 5x6
	WinLines     []WinLine // Выигрышные линии
	ScatterCount int      // Количество скаттеров
	ScatterWin   int64    // Выигрыш от скаттеров
	TotalPayout  int64    // Общий выигрыш
	IsWin        bool     // Есть ли выигрыш
	FreeSpins    int      // Бесплатные спины от скаттеров
}

// WinLine — выигрышная линия.
type WinLine struct {
	LineIndex int    // Номер линии (0-19)
	Symbol    string // Выигрышный символ
	Count     int    // Сколько совпало (3, 4 или 5)
	Payout    int64  // Выплата по этой линии
}

// PayoutTable — таблица выплат (множители от ставки).
var PayoutTable = map[int]int64{
	3: 2,  // 3 символа: 2x ставки (100 пленок)
	4: 5,  // 4 символа: 5x (250 пленок)
	5: 20, // 5 символов: 20x (1000 пленок)
}

// SpecialPayouts — специальные выплаты для редких символов.
var SpecialPayouts = map[string]map[int]int64{
	"7️⃣": {5: 50}, // 5x Seven: 50x (2500 пленок) — ДЖЕКПОТ!
	"💎": {5: 30}, // 5x Diamond: 30x (1500 пленок)
}

// ScatterPayouts — бонусы за скаттеры (появляются в любом месте сетки).
var ScatterPayouts = map[int]struct {
	FreeSpins int
	Bonus     int64
}{
	3: {FreeSpins: 1, Bonus: 100},  // 3 скаттера: 1 фриспин + 100
	4: {FreeSpins: 2, Bonus: 200},  // 4 скаттера: 2 фриспина + 200
	5: {FreeSpins: 3, Bonus: 500},  // 5 скаттеров: 3 фриспина + 500
}
