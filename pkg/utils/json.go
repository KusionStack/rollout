package utils

import "strings"

func Escape(key string) string {
	return strings.ReplaceAll(
		strings.ReplaceAll(key, "~", "~0"), "/", "~1",
	)
}
