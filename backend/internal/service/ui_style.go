package service

import "strings"

const DefaultUIStyle = "classic"

var validUIStyles = map[string]struct{}{
	"classic": {}, "ink": {}, "ocean": {}, "aurora": {}, "sunset": {},
	"forest": {}, "rose": {}, "midnight": {}, "citrus": {}, "slate": {},
}

func IsValidUIStyle(value string) bool {
	_, ok := validUIStyles[strings.TrimSpace(value)]
	return ok
}

func NormalizeUIStyle(value string) string {
	value = strings.TrimSpace(value)
	if IsValidUIStyle(value) {
		return value
	}
	return DefaultUIStyle
}
