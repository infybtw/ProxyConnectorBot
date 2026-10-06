// Package config loads application configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime settings of the application.
type Config struct {
	// TelegramBotToken is the token of the Telegram bot from @BotFather.
	TelegramBotToken string
	// DatabaseURL is the Postgres connection string.
	DatabaseURL string
	// PublicBaseURL is the public base URL of the HTTP service, e.g.
	// "https://sub.example.com". It is used to build subscription links.
	PublicBaseURL string
	// HTTPAddr is the listen address of the fiber server.
	HTTPAddr string
	// DefaultLocale is the locale used for users without a saved choice.
	DefaultLocale string

	// HWIDDeviceOS is the value of the x-device-os header sent to origin.
	HWIDDeviceOS string
	// HWIDVerOS is the value of the x-ver-os header sent to origin.
	HWIDVerOS string
	// HWIDDeviceModel is the value of the x-device-model header sent to origin.
	HWIDDeviceModel string
	// OriginUserAgent is the User-Agent sent to origin.
	OriginUserAgent string

	// OriginTimeout limits a single request to the origin server.
	OriginTimeout time.Duration
	// OriginMaxBody limits the size of a subscription body, in bytes.
	OriginMaxBody int64
}

// Load reads the configuration from environment variables and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		PublicBaseURL:    strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		HTTPAddr:         getenv("HTTP_ADDR", ":8080"),
		DefaultLocale:    getenv("DEFAULT_LOCALE", "ru"),

		HWIDDeviceOS:    getenv("HWID_DEVICE_OS", "android"),
		HWIDVerOS:       getenv("HWID_VER_OS", "14"),
		HWIDDeviceModel: getenv("HWID_DEVICE_MODEL", "Pixel 7"),
		OriginUserAgent: getenv("ORIGIN_USER_AGENT", "Happ/2.4.1 (Android 14)"),

		OriginTimeout: 20 * time.Second,
		OriginMaxBody: 20 << 20,
	}

	if v := os.Getenv("ORIGIN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("ORIGIN_TIMEOUT: %w", err)
		}
		cfg.OriginTimeout = d
	}

	var missing []string
	if cfg.TelegramBotToken == "" {
		missing = append(missing, "TELEGRAM_BOT_TOKEN")
	}
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.PublicBaseURL == "" {
		missing = append(missing, "PUBLIC_BASE_URL")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
