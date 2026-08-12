package conf

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/spf13/viper"
)

type Server struct {
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	JWTSecret string `mapstructure:"jwt_secret"`
	// TrustedProxies 信任的反向代理网段（CIDR/IP），用于从 X-Forwarded-For 等
	// 头解析真实客户端 IP。默认不信任任何代理，此时限流按直连 RemoteAddr 计。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type Log struct {
	Level string `mapstructure:"level"`
}

type Database struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

type Config struct {
	Server   Server   `mapstructure:"server"`
	Log      Log      `mapstructure:"log"`
	Database Database `mapstructure:"database"`
}

var AppConfig Config

func Load(path string) error {
	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("json")
		viper.AddConfigPath("data")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix(APP_NAME)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults()

	if err := viper.ReadInConfig(); err == nil {
		log.Infof("Using config file: %s", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Infof("Config file not found, creating default config")
			if err := os.MkdirAll("data", 0755); err != nil {
				return fmt.Errorf("failed to create data directory: %w", err)
			}
			if err := viper.SafeWriteConfigAs("data/config.json"); err != nil {
				return fmt.Errorf("failed to create default config: %w", err)
			}
		} else {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		return fmt.Errorf("unable to decode config into struct: %w", err)
	}

	if err := ensureJWTSecret(); err != nil {
		return err
	}
	return nil
}

// ensureJWTSecret 在配置中无 jwt_secret 时生成并写回配置文件。
func ensureJWTSecret() error {
	if strings.TrimSpace(AppConfig.Server.JWTSecret) != "" {
		return nil
	}
	secret, err := generateJWTSecret()
	if err != nil {
		return fmt.Errorf("failed to generate jwt secret: %w", err)
	}
	AppConfig.Server.JWTSecret = secret
	viper.Set("server.jwt_secret", secret)
	if err := viper.WriteConfig(); err != nil {
		// 配置文件可能只读；仍允许内存中使用本次生成的 secret
		log.Warnf("failed to persist jwt_secret to config: %v", err)
	} else {
		log.Infof("generated and saved server.jwt_secret")
	}
	return nil
}

func generateJWTSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// JWTSecret 返回用于签发/校验管理端 JWT 的密钥字节。
func JWTSecret() ([]byte, error) {
	s := strings.TrimSpace(AppConfig.Server.JWTSecret)
	if s == "" {
		return nil, fmt.Errorf("jwt secret not configured")
	}
	return []byte(s), nil
}

func setDefaults() {
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.jwt_secret", "")
	viper.SetDefault("server.trusted_proxies", []string{})
	viper.SetDefault("database.type", "sqlite")
	viper.SetDefault("database.path", "data/data.db")
	viper.SetDefault("log.level", "info")
}
