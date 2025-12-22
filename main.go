package main

import (
"fmt"
"log"
"net/http"
"time"
)

func main() {
// Register handlers
http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "text/plain")
fmt.Fprintf(w, " Wisdom House Backend\n")
fmt.Fprintf(w, "======================\n")
fmt.Fprintf(w, "Status: Running \n")
fmt.Fprintf(w, "Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
fmt.Fprintf(w, "Port: 8080\n")
fmt.Fprintf(w, "\nEndpoints:\n")
fmt.Fprintf(w, "  GET  /health         Health check\n")
fmt.Fprintf(w, "  GET  /api/v1/ping    Test endpoint\n")
fmt.Fprintf(w, "  GET  /api/v1/users   Get users\n")
fmt.Fprintf(w, "  POST /api/v1/auth    Authentication\n")
})

http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
fmt.Fprintf(w, `{
"status": "healthy",
"service": "wisdom-house-backend",
"timestamp": "%s",
"version": "1.0.0"
}`, time.Now().Format(time.RFC3339))
})

http.HandleFunc("/api/v1/ping", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
fmt.Fprintf(w, `{
"message": "pong",
"timestamp": %d,
"status": "success"
}`, time.Now().Unix())
})

http.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
fmt.Fprintf(w, `{
"message": "Users endpoint",
"data": [],
"count": 0,
"status": "implemented"
}`)
})

http.HandleFunc("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
if r.Method != "POST" {
http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
return
}
w.Header().Set("Content-Type", "application/json")
fmt.Fprintf(w, `{
"message": "Registration endpoint ready",
"status": "success"
}`)
})

// Start server
port := ":8080"
log.Printf(" Starting Wisdom House Backend...")
log.Printf(" Listening on http://localhost%s", port)
log.Printf("  Health check: http://localhost%s/health", port)
log.Printf(" API Base: http://localhost%s/api/v1", port)

if err := http.ListenAndServe(port, nil); err != nil {
log.Fatal("Server error:", err)
}
}
