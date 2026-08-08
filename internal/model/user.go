package model

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID                 uint   `gorm:"primaryKey"`
	Username           string `gorm:"unique"`
	Password           string `gorm:"not null"`
	MustChangePassword bool   `gorm:"not null;default:false" json:"must_change_password"`
	// TokenVersion 在改密时递增；JWT 携带该版本，不匹配则视为失效。
	TokenVersion int `gorm:"not null;default:0" json:"-"`
}

type UserLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Expire   int    `json:"expire"`
}

type UserChangePassword struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type UserChangeUsername struct {
	NewUsername string `json:"new_username"`
}

type UserLoginResponse struct {
	Token              string `json:"token"`
	ExpireAt           string `json:"expire_at"`
	MustChangePassword bool   `json:"must_change_password"`
}

type UserStatusResponse struct {
	Username           string `json:"username"`
	MustChangePassword bool   `json:"must_change_password"`
}

func (u *User) HashPassword() error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	u.Password = string(hashedPassword)
	return nil
}

func (u *User) ComparePassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
}
