package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all collector configuration loaded from environment variables.
type Config struct {
	// HTTP server
	HTTPAddr            string        `envconfig:"HTTP_ADDR"             default:":8080"`
	ShutdownTimeout     time.Duration `envconfig:"SHUTDOWN_TIMEOUT"      default:"30s"`
	MaxRequestBodyBytes    int64         `envconfig:"MAX_REQUEST_BODY_BYTES"      default:"10485760"` // 10 MB
	MaxConcurrentIngestion int           `envconfig:"MAX_CONCURRENT_INGESTION"    default:"100"`

	// Postgres
	DatabaseURL     string        `envconfig:"DATABASE_URL"      required:"true"`
	DBMaxConns      int32         `envconfig:"DB_MAX_CONNS"       default:"20"`
	DBMinConns      int32         `envconfig:"DB_MIN_CONNS"       default:"2"`
	DBConnTimeout   time.Duration `envconfig:"DB_CONN_TIMEOUT"    default:"10s"`

	// Auth sessions
	SessionDuration time.Duration `envconfig:"SESSION_DURATION" default:"720h"` // 30 days
	APIKeyPepper    string        `envconfig:"API_KEY_PEPPER"   required:"true"`

	// Email / SMTP
	SMTPHost     string `envconfig:"SMTP_HOST"     default:"localhost"`
	SMTPPort     int    `envconfig:"SMTP_PORT"     default:"1025"`
	SMTPUsername string `envconfig:"SMTP_USERNAME"`
	SMTPPassword string `envconfig:"SMTP_PASSWORD"`
	EmailFrom    string `envconfig:"EMAIL_FROM"    default:"noreply@lantern.local"`

	// App
	BaseURL string `envconfig:"BASE_URL" default:"http://localhost:8080"`
	Env     string `envconfig:"ENV"      default:"development"`
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("LANTERN", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
