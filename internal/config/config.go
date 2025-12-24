package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Database DatabaseConfig
	Server   ServerConfig
	Redis    RedisConfig
	SMTP     SMTPConfig
	CORS     CORSConfig
	JWT      JWTConfig
	App      AppConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type ServerConfig struct {
	Port    string
	GinMode string
}

type RedisConfig struct {
	URL      string
	Password string
}

type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

type CORSConfig struct {
	AllowedOrigins []string
}

type JWTConfig struct {
	Secret string
}

type AppConfig struct {
	Environment string
	LogLevel    string
}

func Load() (*Config, error) {
	// Try to load .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("⚠️ No .env file found, using environment variables")
	}

	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "postgres"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "wisdom_church_db"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Server: ServerConfig{
			Port:    getEnv("PORT", "8080"),
			GinMode: getEnv("GIN_MODE", "debug"),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://redis:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
		},
		SMTP: SMTPConfig{
			Host: getEnv("SMTP_HOST", ""),
			Port: getEnv("SMTP_PORT", "587"),
			User: getEnv("SMTP_USER", ""),
			Pass: getEnv("SMTP_PASS", ""),
			From: getEnv("SMTP_FROM", ""),
		},
		CORS: CORSConfig{
			AllowedOrigins: strings.Split(getEnv("ALLOWED_ORIGINS", "http://localhost:3000"), ","),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", ""),
		},
		App: AppConfig{
			Environment: getEnv("ENVIRONMENT", "development"),
			LogLevel:    getEnv("LOG_LEVEL", "info"),
		},
	}, nil
}

func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}