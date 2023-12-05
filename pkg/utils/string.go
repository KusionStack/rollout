package utils

// Abbreviate abbreviates a string using ellipses
func Abbreviate(str string, maxLength uint32) string {
	if maxLength == 0 {
		return ""
	}
	if len(str) <= int(maxLength) {
		return str
	}
	return str[:maxLength] + "..."
}
