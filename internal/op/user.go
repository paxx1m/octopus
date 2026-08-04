package op

import (
	"fmt"
	"sync"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"golang.org/x/crypto/bcrypt"
)

var userCache model.User
var userCacheLock sync.RWMutex

func UserInit() error {
	userCacheLock.Lock()
	defer userCacheLock.Unlock()

	if err := db.GetDB().First(&userCache).Error; err == nil {
		return nil
	}
	userCache.Username = "admin"
	userCache.Password = "admin"
	if err := userCache.HashPassword(); err != nil {
		return err
	}
	if err := db.GetDB().Create(&userCache).Error; err != nil {
		return err
	}
	log.Infof("initial user: admin,password: admin")
	return nil
}

func UserChangePassword(oldPassword, newPassword string) error {
	userCacheLock.Lock()
	defer userCacheLock.Unlock()

	if err := userCache.ComparePassword(oldPassword); err != nil {
		return fmt.Errorf("incorrect old password: %w", err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}
	hashedStr := string(hashed)

	if err := db.GetDB().Model(&model.User{}).Where("id = ?", userCache.ID).
		Update("password", hashedStr).Error; err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	userCache.Password = hashedStr
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

	if username != userCache.Username {
		return fmt.Errorf("incorrect username")
	}
	if err := userCache.ComparePassword(password); err != nil {
		return fmt.Errorf("incorrect password")
	}
	return nil
}

func UserGet() model.User {
	userCacheLock.RLock()
	defer userCacheLock.RUnlock()
	return userCache
}
