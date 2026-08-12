package db

import (
	"errors"

	"gorm.io/gorm"
)

// IsDuplicateError reports whether err is a UNIQUE / duplicate-key constraint failure.
// 依赖 gorm.Config{TranslateError: true} 的语义化错误，不匹配驱动文案。
func IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
