package utils

// Abbreviate abbreviates a string using ellipses
func Abbreviate(str string, maxLength uint32) string {
	if len(str) <= int(maxLength) {
		return str
	}
	return str[:maxLength] + "..."
}
