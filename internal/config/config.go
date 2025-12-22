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
    CORS     CORSConfig
    JWT      JWTConfig
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

type CORSConfig struct {
    AllowedOrigins []string
}

type JWTConfig struct {
    Secret string
}

func Load() (*Config, error) {
    if err := godotenv.Load(); err != nil {
        fmt.Println("No .env file found, using environment variables")
    }

    config := &Config{
        Database: DatabaseConfig{
            Host:     getEnv("DB_HOST", "localhost"),
            Port:     getEnv("DB_PORT", "5432"),
            User:     getEnv("DB_USER", "postgres"),
            Password: getEnv("DB_PASSWORD", ""),
            DBName:   getEnv("DB_NAME", "church_db"),
            SSLMode:  getEnv("DB_SSLMODE", "disable"),
        },
        Server: ServerConfig{
            Port:    getEnv("PORT", "8080"),
            GinMode: getEnv("GIN_MODE", "debug"),
        },
        CORS: CORSConfig{
            AllowedOrigins: strings.Split(getEnv("ALLOWED_ORIGINS", "http://localhost:3000"), ","),
        },
        JWT: JWTConfig{
            Secret: getEnv("JWT_SECRET", "default-secret-change-me"),
        },
    }

    return config, nil
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