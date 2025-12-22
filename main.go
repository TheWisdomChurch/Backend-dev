package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
	
	"github.com/joho/godotenv"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
	Port      string `json:"port"`
}

type PingResponse struct {
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

type UserResponse struct {
	Message string        `json:"message"`
	Data    []interface{} `json:"data"`
	Count   int           `json:"count"`
	Status  string        `json:"status"`
}

type Config struct {
	Port     string
	DBHost   string
	DBPort   string
	DBName   string
	DBUser   string
	DBPass   string
}

func loadConfig() *Config {
	// Try to load .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()
	
	return &Config{
		Port:     getEnv("PORT", "8080"),
		DBHost:   getEnv("DB_HOST", ""),
		DBPort:   getEnv("DB_PORT", "5432"),
		DBName:   getEnv("DB_NAME", ""),
		DBUser:   getEnv("DB_USER", ""),
		DBPass:   getEnv("DB_PASSWORD", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func main() {
	config := loadConfig()
	
	// Log configuration (mask password in logs)
	maskedPass := ""
	if config.DBPass != "" {
		maskedPass = "****"
	}
	
	log.Printf("🚀 Starting Wisdom House Backend v1.0.0")
	log.Printf("📡 Server will listen on port: %s", config.Port)
	log.Printf("🗄️  Database config: %s:%s/%s (user: %s, pass: %s)", 
		config.DBHost, config.DBPort, config.DBName, config.DBUser, maskedPass)
	// Setup routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Wisdom House Backend API\n")
		fmt.Fprintf(w, "========================\n")
		fmt.Fprintf(w, "Version: 1.0.0\n")
		fmt.Fprintf(w, "Status: Operational\n")
		fmt.Fprintf(w, "Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		fmt.Fprintf(w, "Port: %s\n", config.Port)
		fmt.Fprintf(w, "\nAvailable Endpoints:\n")
		fmt.Fprintf(w, "  GET  /health               Health check\n")
		fmt.Fprintf(w, "  GET  /api/v1/ping          API test\n")
		fmt.Fprintf(w, "  GET  /api/v1/users         Users endpoint\n")
		fmt.Fprintf(w, "  POST /api/v1/auth/register User registration\n")
		fmt.Fprintf(w, "\nDocumentation: Coming soon\n")
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		response := HealthResponse{
			Status:    "healthy",
			Service:   "wisdom-house-backend",
			Timestamp: time.Now().Format(time.RFC3339),
			Version:   "1.0.0",
			Port:      config.Port,
		}
		writeJSON(w, http.StatusOK, response)
	})

	http.HandleFunc("/api/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		response := PingResponse{
			Message:   "pong",
			Timestamp: time.Now().Unix(),
			Status:    "success",
		}
		writeJSON(w, http.StatusOK, response)
	})

	http.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		response := UserResponse{
			Message: "Users endpoint - ready for implementation",
			Data:    []interface{}{},
			Count:   0,
			Status:  "implemented",
		}
		writeJSON(w, http.StatusOK, response)
	})

	http.HandleFunc("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "Method not allowed",
			})
			return
		}
		
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "Registration endpoint ready for implementation",
			"status":  "success",
		})
	})

	// Graceful shutdown setup
	server := &http.Server{
		Addr:         ":" + config.Port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("✅ Server is ready: http://localhost:%s", config.Port)
	log.Printf("📊 Health check: http://localhost:%s/health", config.Port)
	log.Printf("⚡ Press Ctrl+C to stop the server\n")
	
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}