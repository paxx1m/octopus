package op

import (
	"errors"
	"fmt"
	"sync"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var userCache model.User
var userCacheLock sync.RWMutex

func UserInit() error {
	userCacheLock.Lock()
	defer userCacheLock.Unlock()

	err := db.GetDB().First(&userCache).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// 数据库故障等真实错误不能当作「无用户」而创建默认账号
		return fmt.Errorf("failed to load user: %w", err)
	}
	if err == nil {
		// 已有用户仍使用默认密码 admin 时，强制下次改密
		if !userCache.MustChangePassword && userCache.ComparePassword("admin") == nil {
			if err := db.GetDB().Model(&model.User{}).Where("id = ?", userCache.ID).
				Update("must_change_password", true).Error; err != nil {
				log.Warnf("failed to mark must_change_password: %v", err)
			} else {
				userCache.MustChangePassword = true
				log.Warnf("default password detected; must change password on next login")
			}
		}
		return nil
	}
	userCache.Username = "admin"
	userCache.Password = "admin"
	userCache.MustChangePassword = true
	if err := userCache.HashPassword(); err != nil {
		return err
	}
	if err := db.GetDB().Create(&userCache).Error; err != nil {
		return err
	}
	log.Infof("initial user: admin / admin (must change password on first login)")
	return nil
}

func UserChangePassword(oldPassword, newPassword string) error {
	userCacheLock.Lock()
	defer userCacheLock.Unlock()

	if err := userCache.ComparePassword(oldPassword); err != nil {
		return fmt.Errorf("incorrect old password")
	}
	if newPassword == "" {
		return fmt.Errorf("new password is required")
	}
	if newPassword == oldPassword {
		return fmt.Errorf("new password must differ from old password")
	}
	// 默认 admin/admin 场景：强制改密时禁止继续使用 admin
	if userCache.MustChangePassword && newPassword == "admin" {
		return fmt.Errorf("please choose a password other than the default")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}
	hashedStr := string(hashed)

	nextVer := userCache.TokenVersion + 1
	if err := db.GetDB().Model(&model.User{}).Where("id = ?", userCache.ID).
		Updates(map[string]any{
			"password":             hashedStr,
			"must_change_password": false,
			"token_version":        nextVer,
		}).Error; err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	userCache.Password = hashedStr
	userCache.MustChangePassword = false
	userCache.TokenVersion = nextVer
	return nil
}

func UserChangeUsername(newUsername string) error {
	userCacheLock.Lock()
	defer userCacheLock.Unlock()

	if userCache.Username == newUsername {
		return fmt.Errorf("new username is the same as the old username")
	}
	if err := db.GetDB().Model(&model.User{}).Where("id = ?", userCache.ID).
		Update("username", newUsername).Error; err != nil {
		return fmt.Errorf("failed to update username: %w", err)
	}
	userCache.Username = newUsername
	return nil
}

func UserVerify(username, password string) error {
	userCacheLock.RLock()
	defer userCacheLock.RUnlock()

	// 统一错误文案，避免用户名枚举
	if username != userCache.Username {
		return fmt.Errorf("invalid username or password")
	}
	if err := userCache.ComparePassword(password); err != nil {
		return fmt.Errorf("invalid username or password")
	}
	return nil
}

func UserGet() model.User {
	userCacheLock.RLock()
	defer userCacheLock.RUnlock()
	return userCache
}

func UserMustChangePassword() bool {
	userCacheLock.RLock()
	defer userCacheLock.RUnlock()
	return userCache.MustChangePassword
}

// UserTokenVersion 返回当前 token 版本，供 JWT 签发/校验。
func UserTokenVersion() int {
	userCacheLock.RLock()
	defer userCacheLock.RUnlock()
	return userCache.TokenVersion
}
