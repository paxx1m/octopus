package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/golang-jwt/jwt/v5"
)

// adminClaims 管理端 JWT claims。
// TokenVer 与用户 TokenVersion 绑定，改密后旧 token 立即失效。
type adminClaims struct {
	TokenVer int `json:"tv"`
	jwt.RegisteredClaims
}

// GenerateJWTToken 签发管理端 JWT。
// expiresMin: 0=15 分钟；>0 为分钟数；-1=30 天。
func GenerateJWTToken(expiresMin int) (string, string, error) {
	now := time.Now()
	claims := &adminClaims{
		TokenVer: op.UserTokenVersion(),
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    conf.APP_NAME,
		},
	}
	if expiresMin == 0 {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(15 * time.Minute))
	} else if expiresMin > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(time.Duration(expiresMin) * time.Minute))
	} else if expiresMin == -1 {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(30 * 24 * time.Hour))
	} else {
		return "", "", fmt.Errorf("invalid expire value")
	}

	secret, err := conf.JWTSecret()
	if err != nil {
		return "", "", err
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", "", err
	}
	return token, claims.ExpiresAt.Format(time.RFC3339), nil
}

// VerifyJWTToken 校验 JWT：必须 HS256，签名匹配，且 token 版本与当前用户一致。
func VerifyJWTToken(token string) bool {
	secret, err := conf.JWTSecret()
	if err != nil {
		return false
	}
	claims := &adminClaims{}
	jwtToken, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method == nil || t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !jwtToken.Valid {
		return false
	}
	if claims.TokenVer != op.UserTokenVersion() {
		return false
	}
	return true
}

func GenerateAPIKey() string {
	const keyChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 48)
	maxI := big.NewInt(int64(len(keyChars)))
	for i := range b {
		n, err := rand.Int(rand.Reader, maxI)
		if err != nil {
			return ""
		}
		b[i] = keyChars[n.Int64()]
	}
	return "sk-" + conf.APP_NAME + "-" + string(b)
}
