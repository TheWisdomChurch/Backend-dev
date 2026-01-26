// internal/config/config.go
package config

import (
	"fmt"
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

// Enabled returns true only when Bunny is fully configured enough to be usable.
// This prevents partial envs from causing startup failure.
func (b BunnyConfig) Enabled() bool {
	return strings.TrimSpace(b.StorageZone) != "" &&
		strings.TrimSpace(b.StorageKey) != "" &&
		strings.TrimSpace(b.PullZone) != ""
}

// Validate enforces required fields ONLY if Bunny is intended to be enabled.
// If Bunny is not enabled, config is valid and uploads should be treated as disabled.
func (b BunnyConfig) Validate() error {
	zone := strings.TrimSpace(b.StorageZone)
	key := strings.TrimSpace(b.StorageKey)
	region := strings.TrimSpace(b.StorageRegion)
	pull := strings.TrimRight(strings.TrimSpace(b.PullZone), "/")

	// If none of the Bunny “core” fields are set, treat as disabled and accept.
	if zone == "" && key == "" && pull == "" {
		return nil
	}

	// If some are set, require all core + region.
	if zone == "" || key == "" || pull == "" {
		return fmt.Errorf("BunnyCDN config incomplete: require BUNNYCDN_STORAGE_ZONE, BUNNYCDN_STORAGE_KEY, BUNNYCDN_PULL_ZONE (and optionally BUNNYCDN_STORAGE_REGION)")
	}

	// region is required once Bunny is enabled; if not provided, we default to "de".
	_ = region
	return nil
}

type DatabaseConfig struct {
	Host     string `json:"host" env:"DATABASE_HOST,required"`
	Port     string `json:"port" env:"DATABASE_PORT,required"`
	User     string `json:"user" env:"DATABASE_USERNAME,required"`
	Password string `json:"-" env:"DATABASE_PASSWORD,required"`
	DBName   string `json:"dbname" env:"DATABASE_DBNAME,required"`
	SSLMode  string `json:"sslmode" env:"DATABASE_SSLMODE"`

	MaxIdleConns    int           `json:"max_idle_conns" env:"DATABASE_MAX_IDLE_CONNS"`
	MaxOpenConns    int           `json:"max_open_conns" env:"DATABASE_MAX_OPEN_CONNS"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" env:"DATABASE_CONN_MAX_LIFETIME"`
}

type ServerConfig struct {
	Port           string        `json:"port" env:"SERVER_PORT,required"`
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
	Secret     string        `json:"-" env:"JWT_SECRET,required"`
	Expiration time.Duration `json:"expiration" env:"JWT_EXPIRATION"`
}

type AppConfig struct {
	Environment    string `json:"environment" env:"ENVIRONMENT"`
	LogLevel       string `json:"log_level" env:"LOG_LEVEL"`
	Name           string `json:"name" env:"APP_NAME"`
	Version        string `json:"version" env:"APP_VERSION"`
	Debug          bool   `json:"debug" env:"APP_DEBUG"`
	PublicURL      string `json:"public_url" env:"APP_PUBLIC_URL"`
	FrontendURL    string `json:"frontend_url" env:"APP_FRONTEND_URL"`
	LogoURL        string `json:"logo_url" env:"APP_LOGO_URL"`
	PastorName     string `json:"pastor_name" env:"APP_PASTOR_NAME"`
	SupportEmail   string `json:"support_email" env:"APP_SUPPORT_EMAIL"`
	AdminPortalURL string `json:"admin_portal_url" env:"APP_ADMIN_PORTAL_URL"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Database: DatabaseConfig{
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
			URL:          getEnv("REDIS_URL", "redis://localhost:6379"),
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
			Environment:    getEnv("ENVIRONMENT", "development"),
			LogLevel:       getEnv("LOG_LEVEL", "info"),
			Name:           getEnv("APP_NAME", "Wisdom House Backend"),
			Version:        getEnv("APP_VERSION", "1.0.0"),
			Debug:          getEnvAsBool("APP_DEBUG", false),
			PublicURL:      getEnv("APP_PUBLIC_URL", "http://localhost:8080"),
			FrontendURL:    getEnv("APP_FRONTEND_URL", "http://localhost:3000"),
			LogoURL:        getEnv("APP_LOGO_URL", ""),
			PastorName:     getEnv("APP_PASTOR_NAME", "Senior Pastor"),
			SupportEmail:   getEnv("APP_SUPPORT_EMAIL", ""),
			AdminPortalURL: getEnv("APP_ADMIN_PORTAL_URL", ""),
		},
		Bunny: BunnyConfig{
			StorageZone:   getEnv("BUNNYCDN_STORAGE_ZONE", ""),
			StorageKey:    getEnv("BUNNYCDN_STORAGE_KEY", ""),
			StorageRegion: getEnv("BUNNYCDN_STORAGE_REGION", "de"), // default OK, does NOT enable Bunny
			PullZone:      strings.TrimRight(getEnv("BUNNYCDN_PULL_ZONE", ""), "/"),
			BasePath:      strings.Trim(strings.TrimSpace(getEnv("BUNNYCDN_BASE_PATH", "uploads")), "/"),
		},
	}

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// ConnectionString returns PostgreSQL connection string
func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// DSN returns formatted DSN without password (for logging)
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s@%s:%s/%s?sslmode=%s",
		c.User, c.Host, c.Port, c.DBName, c.SSLMode,
	)
}

func validateConfig(cfg *Config) error {
	// JWT secret required always (as you designed)
	if cfg.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}

	// Database password required in production
	if cfg.App.Environment == "production" && cfg.Database.Password == "" {
		return fmt.Errorf("DATABASE_PASSWORD is required in production")
	}

	// SMTP sanity (optional)
	if cfg.SMTP.Host != "" {
		if cfg.SMTP.User == "" {
			return fmt.Errorf("SMTP_USER is required when SMTP_HOST is set")
		}
		if cfg.SMTP.Password == "" {
			return fmt.Errorf("SMTP_PASS is required when SMTP_HOST is set")
		}
	}

	// ✅ Bunny optional (validate only if partially/fully set)
	if err := cfg.Bunny.Validate(); err != nil {
		return err
	}

	return nil
}

// Helper functions
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
