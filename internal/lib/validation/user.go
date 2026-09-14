package validation

import (
	"strings"
	"unicode/utf8"
)

func Credentials(userToken, password string) []string {
	var messages []string
	if strings.TrimSpace(userToken) == "" || len(userToken) > 255 {
		messages = append(messages, "user_token must contain 1 to 255 bytes")
	}
	if len(password) < 1 || len(password) > 72 {
		messages = append(messages, "password must contain 1 to 72 bytes")
	}
	return messages
}
func NewUser(userToken, name, password string, isAdmin bool) []string {
	messages := Credentials(userToken, password)
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 255 {
		messages = append(messages, "name must contain 1 to 255 characters")
	}
	isPIN := len(password) == 8 && strings.Trim(password, "0123456789") == ""
	if len(password) < 5 && (isAdmin || !isPIN) {
		messages = append(messages, "new password must contain at least 8 bytes (non-admin users may use a 5-digit PIN)")
	}
	return messages
}
