package utils

func Ellipsis(text string, length int) string {
	runes := []rune(text)
	if len(runes) <= length {
		return text
	}
	return string(runes[:length - 1]) + "…"
}
