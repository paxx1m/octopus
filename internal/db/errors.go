package db

import "strings"

// IsDuplicateError reports whether err is a UNIQUE / duplicate-key constraint failure
// from SQLite, MySQL, or PostgreSQL (message-based; portable without driver-specific types).
func IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "duplicate key value")
}
