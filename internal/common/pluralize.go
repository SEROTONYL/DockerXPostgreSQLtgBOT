package common

import "fmt"

func FormatFilmsAmount(amount int64) string {
	if amount >= 0 {
		return fmt.Sprintf("+%d%s", amount, FilmFramesEmoji)
	}
	return fmt.Sprintf("%d%s", amount, FilmFramesEmoji)
}

func FormatNumber(n int64) string {
	if n < 0 {
		return "-" + FormatNumber(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	rest := n / 1000
	last := n % 1000

	if rest > 0 {
		return fmt.Sprintf("%s %03d", FormatNumber(rest), last)
	}
	return fmt.Sprintf("%d", last)
}
