// internal/config/config.go
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Database DatabaseConfig `json:"database"`
	Server   ServerConfig   `json:"server"`
	Redis    RedisConfig    `json:"redis"`
	SMTP     SMTPConfig     `json:"smtp"`
	CORS     CORSConfig     `json:"cors"`
	JWT      JWTConfig      `json:"jwt"`
	App      AppConfig      `json:"app"`

	// BunnyCDN / Bunny Storage config (OPTIONAL)
	Bunny BunnyConfig `json:"bunny"`
}

type BunnyConfig struct {
	StorageZone   string `json:"storage_zone" env:"BUNNYCDN_STORAGE_ZONE"`
	StorageKey    string `json:"-" env:"BUNNYCDN_STORAGE_KEY"`
	StorageRegion string `json:"storage_region" env:"BUNNYCDN_STORAGE_REGION"`
	PullZone      string `json:"pull_zone" env:"BUNNYCDN_PULL_ZONE"`
	BasePath      string `json:"base_path" env:"BUNNYCDN_BASE_PATH"`
}

func (b BunnyConfig) Enabled() bool {
	return strings.TrimSpace(b.StorageZone) != "" &&
		strings.TrimSpace(b.StorageKey) != "" &&
		strings.TrimSpace(b.PullZone) != ""
}

func (b BunnyConfig) Validate() error {
	zone := strings.TrimSpace(b.StorageZone)
	key := strings.TrimSpace(b.StorageKey)
	pull := strings.TrimRight(strings.TrimSpace(b.PullZone), "/")

	// If none of the core fields are set, treat as disabled.
	if zone == "" && key == "" && pull == "" {
		return nil
	}
	if zone == "" || key == "" || pull == "" {
		return fmt.Errorf("BunnyCDN config incomplete: require BUNNYCDN_STORAGE_ZONE, BUNNYCDN_STORAGE_KEY, BUNNYCDN_PULL_ZONE (and optionally BUNNYCDN_STORAGE_REGION)")
	}
	return nil
}

type DatabaseConfig struct {
	// Prefer DATABASE_URL (Supabase / managed Postgres)
	URL string `json:"url" env:"DATABASE_URL"`

	// Fallback: parts-based (local/docker Postgres)
	Host     string `json:"host" env:"DATABASE_HOST"`
	Port     string `json:"port" env:"DATABASE_PORT"`
	User     string `json:"user" env:"DATABASE_USERNAME"`
	Password string `json:"-" env:"DATABASE_PASSWORD"`
	DBName   string `json:"dbname" env:"DATABASE_DBNAME"`
	SSLMode  string `json:"sslmode" env:"DATABASE_SSLMODE"`

	MaxIdleConns    int           `json:"max_idle_conns" env:"DATABASE_MAX_IDLE_CONNS"`
	MaxOpenConns    int           `json:"max_open_conns" env:"DATABASE_MAX_OPEN_CONNS"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" env:"DATABASE_CONN_MAX_LIFETIME"`
}

type ServerConfig struct {
	Port           string        `json:"port" env:"SERVER_PORT"`
	GinMode        string        `json:"gin_mode" env:"SERVER_GIN_MODE"`
	ReadTimeout    time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT"`
	WriteTimeout   time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT"`
	MaxHeaderBytes int           `json:"max_header_bytes" env:"SERVER_MAX_HEADER_BYTES"`
}

type RedisConfig struct {
	URL          string        `json:"url" env:"REDIS_URL"`
	Password     string        `json:"-" env:"REDIS_PASSWORD"`
	DB           int           `json:"db" env:"REDIS_DB"`
	PoolSize     int           `json:"pool_size" env:"REDIS_POOL_SIZE"`
	MinIdleConns int           `json:"min_idle_conns" env:"REDIS_MIN_IDLE_CONNS"`
	DialTimeout  time.Duration `json:"dial_timeout" env:"REDIS_DIAL_TIMEOUT"`
	ReadTimeout  time.Duration `json:"read_timeout" env:"REDIS_READ_TIMEOUT"`
	WriteTimeout time.Duration `json:"write_timeout" env:"REDIS_WRITE_TIMEOUT"`
	PoolTimeout  time.Duration `json:"pool_timeout" env:"REDIS_POOL_TIMEOUT"`
	IdleTimeout  time.Duration `json:"idle_timeout" env:"REDIS_IDLE_TIMEOUT"`
}

type SMTPConfig struct {
	Host     string `json:"host" env:"SMTP_HOST"`
	Port     string `json:"port" env:"SMTP_PORT"`
	User     string `json:"user" env:"SMTP_USER"`
	Password string `json:"-" env:"SMTP_PASS"`
	From     string `json:"from" env:"SMTP_FROM"`
	TLS      bool   `json:"tls" env:"SMTP_TLS"`
}

type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins" env:"CORS_ALLOW_ORIGIN"`
	AllowedMethods   []string `json:"allowed_methods" env:"CORS_ALLOW_METHODS"`
	AllowedHeaders   []string `json:"allowed_headers" env:"CORS_ALLOW_HEADERS"`
	AllowCredentials bool     `json:"allow_credentials" env:"CORS_ALLOW_CREDENTIALS"`
	ExposedHeaders   []string `json:"exposed_headers" env:"CORS_EXPOSED_HEADERS"`
	MaxAge           int      `json:"max_age" env:"CORS_MAX_AGE"`
}

type JWTConfig struct {
	Secret     string        `json:"-" env:"JWT_SECRET"`
	Expiration time.Duration `json:"expiration" env:"JWT_EXPIRATION"`
}

type AppConfig struct {
	Environment               string        `json:"environment" env:"ENVIRONMENT"` // "development" | "production"
	LogLevel                  string        `json:"log_level" env:"LOG_LEVEL"`
	Name                      string        `json:"name" env:"APP_NAME"`
	Version                   string        `json:"version" env:"APP_VERSION"`
	Debug                     bool          `json:"debug" env:"APP_DEBUG"`
	PublicURL                 string        `json:"public_url" env:"APP_PUBLIC_URL"`
	FrontendURL               string        `json:"frontend_url" env:"APP_FRONTEND_URL"`
	LogoURL                   string        `json:"logo_url" env:"APP_LOGO_URL"`
	PastorName                string        `json:"pastor_name" env:"APP_PASTOR_NAME"`
	SupportEmail              string        `json:"support_email" env:"APP_SUPPORT_EMAIL"`
	AdminPortalURL            string        `json:"admin_portal_url" env:"APP_ADMIN_PORTAL_URL"`
	FormCleanupInterval       time.Duration `json:"form_cleanup_interval" env:"APP_FORM_CLEANUP_INTERVAL"`
	EmailTemplateAssetBaseURL string        `json:"email_template_asset_base_url" env:"APP_EMAIL_TEMPLATE_ASSET_BASE_URL"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	env := strings.ToLower(strings.TrimSpace(getEnv("ENVIRONMENT", "development")))

	cfg := &Config{
		Database: DatabaseConfig{
			URL:             strings.TrimSpace(getEnv("DATABASE_URL", "")),
			Host:            getEnv("DATABASE_HOST", "localhost"),
			Port:            getEnv("DATABASE_PORT", "5432"),
			User:            getEnv("DATABASE_USERNAME", "postgres"),
			Password:        getEnv("DATABASE_PASSWORD", ""),
			DBName:          getEnv("DATABASE_DBNAME", "wisdom_church_db"),
			SSLMode:         getEnv("DATABASE_SSLMODE", "disable"),
			MaxIdleConns:    getEnvAsInt("DATABASE_MAX_IDLE_CONNS", 10),
			MaxOpenConns:    getEnvAsInt("DATABASE_MAX_OPEN_CONNS", 100),
			ConnMaxLifetime: getEnvAsDuration("DATABASE_CONN_MAX_LIFETIME", time.Hour),
		},
		Server: ServerConfig{
			Port:           getEnv("SERVER_PORT", "8080"),
			GinMode:        getEnv("SERVER_GIN_MODE", "debug"),
			ReadTimeout:    getEnvAsDuration("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:   getEnvAsDuration("SERVER_WRITE_TIMEOUT", 10*time.Second),
			MaxHeaderBytes: getEnvAsInt("SERVER_MAX_HEADER_BYTES", 1<<20),
		},
		Redis: RedisConfig{
			URL:          getEnv("REDIS_URL", "redis://redis:6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getEnvAsInt("REDIS_DB", 0),
			PoolSize:     getEnvAsInt("REDIS_POOL_SIZE", 10),
			MinIdleConns: getEnvAsInt("REDIS_MIN_IDLE_CONNS", 5),
			DialTimeout:  getEnvAsDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:  getEnvAsDuration("REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout: getEnvAsDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),
			PoolTimeout:  getEnvAsDuration("REDIS_POOL_TIMEOUT", 4*time.Second),
			IdleTimeout:  getEnvAsDuration("REDIS_IDLE_TIMEOUT", 5*time.Minute),
		},
		SMTP: SMTPConfig{
			Host:     getEnv("SMTP_HOST", ""),
			Port:     getEnv("SMTP_PORT", "587"),
			User:     getEnv("SMTP_USER", ""),
			Password: getEnv("SMTP_PASS", ""),
			From:     getEnv("SMTP_FROM", ""),
			TLS:      getEnvAsBool("SMTP_TLS", true),
		},
		CORS: CORSConfig{
			AllowedOrigins:   splitEnv("CORS_ALLOW_ORIGIN", []string{"http://localhost:3000", "http://localhost:3001"}),
			AllowedMethods:   splitEnv("CORS_ALLOW_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}),
			AllowedHeaders:   splitEnv("CORS_ALLOW_HEADERS", []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"}),
			AllowCredentials: getEnvAsBool("CORS_ALLOW_CREDENTIALS", true),
			ExposedHeaders:   splitEnv("CORS_EXPOSED_HEADERS", []string{"Content-Length", "Content-Range", "X-Total-Count"}),
			MaxAge:           getEnvAsInt("CORS_MAX_AGE", 86400),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", ""),
			Expiration: getEnvAsDuration("JWT_EXPIRATION", 24*time.Hour),
		},
		App: AppConfig{
			Environment:               env,
			LogLevel:                  getEnv("LOG_LEVEL", "info"),
			Name:                      getEnv("APP_NAME", "Wisdom House Backend"),
			Version:                   getEnv("APP_VERSION", "1.0.0"),
			Debug:                     getEnvAsBool("APP_DEBUG", false),
			PublicURL:                 getEnv("APP_PUBLIC_URL", "http://localhost:8080"),
			FrontendURL:               getEnv("APP_FRONTEND_URL", "http://localhost:3000"),
			LogoURL:                   getEnv("APP_LOGO_URL", ""),
			PastorName:                getEnv("APP_PASTOR_NAME", "Senior Pastor"),
			SupportEmail:              getEnv("APP_SUPPORT_EMAIL", ""),
			AdminPortalURL:            getEnv("APP_ADMIN_PORTAL_URL", ""),
			FormCleanupInterval:       getEnvAsDuration("APP_FORM_CLEANUP_INTERVAL", 1*time.Hour),
			EmailTemplateAssetBaseURL: strings.TrimRight(getEnv("APP_EMAIL_TEMPLATE_ASSET_BASE_URL", ""), "/"),
		},
		Bunny: BunnyConfig{
			StorageZone:   getEnv("BUNNYCDN_STORAGE_ZONE", ""),
			StorageKey:    getEnv("BUNNYCDN_STORAGE_KEY", ""),
			StorageRegion: getEnv("BUNNYCDN_STORAGE_REGION", "de"),
			PullZone:      strings.TrimRight(getEnv("BUNNYCDN_PULL_ZONE", ""), "/"),
			BasePath:      strings.Trim(strings.TrimSpace(getEnv("BUNNYCDN_BASE_PATH", "uploads")), "/"),
		},
	}

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}
	return cfg, nil
}

// ConnectionString prefers DATABASE_URL when provided; otherwise uses parts-based DSN.
func (c *DatabaseConfig) ConnectionString() string {
	if strings.TrimSpace(c.URL) != "" {
		return c.URL
	}
	ssl := strings.TrimSpace(c.SSLMode)
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, ssl,
	)
}

// DSN returns a log-safe DSN (password redacted).
func (c *DatabaseConfig) DSN() string {
	raw := strings.TrimSpace(c.URL)
	if raw == "" {
		// parts-based: never print password
		return fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s",
			c.Host, c.Port, c.User, c.DBName, c.SSLMode)
	}
	return redactPostgresURL(raw)
}

func validateConfig(cfg *Config) error {
	if strings.TrimSpace(cfg.JWT.Secret) == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}

	// In production, require DATABASE_URL (recommended),
	// but still allow parts-based config if fully provided.
	if cfg.App.Environment == "production" {
		if strings.TrimSpace(cfg.Database.URL) == "" {
			if strings.TrimSpace(cfg.Database.Host) == "" {
				return fmt.Errorf("DATABASE_HOST is required when DATABASE_URL is not set")
			}
			if strings.TrimSpace(cfg.Database.Port) == "" {
				return fmt.Errorf("DATABASE_PORT is required when DATABASE_URL is not set")
			}
			if strings.TrimSpace(cfg.Database.User) == "" {
				return fmt.Errorf("DATABASE_USERNAME is required when DATABASE_URL is not set")
			}
			if strings.TrimSpace(cfg.Database.DBName) == "" {
				return fmt.Errorf("DATABASE_DBNAME is required when DATABASE_URL is not set")
			}
			if strings.TrimSpace(cfg.Database.Password) == "" {
				return fmt.Errorf("DATABASE_PASSWORD is required when DATABASE_URL is not set")
			}
		}
	}

	if strings.TrimSpace(cfg.SMTP.Host) != "" {
		if strings.TrimSpace(cfg.SMTP.User) == "" {
			return fmt.Errorf("SMTP_USER is required when SMTP_HOST is set")
		}
		if strings.TrimSpace(cfg.SMTP.Password) == "" {
			return fmt.Errorf("SMTP_PASS is required when SMTP_HOST is set")
		}
	}

	if err := cfg.Bunny.Validate(); err != nil {
		return err
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func splitEnv(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}
	return defaultValue
}

func redactPostgresURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		// fallback best-effort
		return raw
	}
	if u.User != nil {
		username := u.User.Username()
		u.User = url.UserPassword(username, "***")
	}
	return u.String()
}
