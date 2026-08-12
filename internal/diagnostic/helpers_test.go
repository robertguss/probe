package diagnostic_test

import (
	"strconv"
	"unicode/utf8"
)

// Shared test helpers (package diagnostic_test).

func itoa(n int) string {
	return strconv.Itoa(n)
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "…"
}
