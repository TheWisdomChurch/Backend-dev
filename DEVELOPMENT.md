# Development Guide

## Setup

### Prerequisites
- Go 1.25+
- PostgreSQL 15
- Redis 7
- Docker & Docker Compose (optional)

### Install Dependencies
```bash
go mod download
go mod tidy
```

### Environment Configuration
```bash
cp .env.example .env
# Edit .env with your local database credentials
```

## Running Locally

### Option 1: With Docker Compose
```bash
docker-compose up -d
go run cmd/api/main.go
```

### Option 2: Manual Setup
```bash
# Start PostgreSQL
psql -U postgres -c "CREATE DATABASE wisdom_house_dev;"

# Start Redis
redis-server

# Run migrations
go run cmd/api/main.go migrate

# Start server
go run cmd/api/main.go
```

## Development Workflow

### Making Changes
```bash
# Create feature branch
git checkout -b feature/my-feature

# Make changes and test
make test

# Run linter
make lint

# Commit changes
git commit -am "Add feature description"

# Push to remote
git push origin feature/my-feature
```

### Testing
```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/service -v

# Run with coverage
make test-coverage
```

### Building
```bash
# Development build
make build

# Production build
make build-prod

# Run binary
./bin/wisdomhousebe
```

## Code Structure

### cmd/api/
Application entry point and server setup

### internal/
Core application code

#### handlers/
HTTP request handlers - organize by domain
```
handlers/
├── auth.go
├── form_handler.go
├── event_handler.go
└── ...
```

#### service/
Business logic - one service per domain
```
service/
├── auth_service.go
├── form_service.go
├── event_service.go
└── ...
```

#### repository/
Database access layer
```
repository/
├── user_repository.go
├── form_repository.go
├── event_repository.go
└── ...
```

#### models/
Data structures
```
models/
├── user.go
├── form.go
├── event.go
└── ...
```

### internal/middleware/
HTTP middleware - authentication, logging, CORS, etc.

### internal/config/
Configuration management

### internal/database/
Database connections and migrations

## API Development

### Adding a New Endpoint

1. **Define the model** (`internal/models/`)
```go
type NewEntity struct {
    ID        string `gorm:"primaryKey"`
    Name      string
    CreatedAt time.Time
}
```

2. **Create repository** (`internal/repository/`)
```go
type NewEntityRepository interface {
    Create(ctx context.Context, entity *models.NewEntity) error
    GetByID(ctx context.Context, id string) (*models.NewEntity, error)
}
```

3. **Implement service** (`internal/service/`)
```go
type NewEntityService interface {
    Create(ctx context.Context, req CreateNewEntityRequest) (*NewEntity, error)
}

type newEntityService struct {
    repo repository.NewEntityRepository
}
```

4. **Create handler** (`internal/handlers/`)
```go
func (h *NewEntityHandler) Create(c *gin.Context) {
    // Validation
    // Call service
    // Return response
}
```

5. **Register route** (`cmd/api/main.go`)
```go
api.POST("/new-entities", newEntityHandler.Create)
```

## Debugging

### Enable Debug Logging
```bash
LOG_LEVEL=debug go run cmd/api/main.go
```

### Debug in IDE
Visual Studio Code with Go extension:
```json
// .vscode/launch.json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Connect to Port",
            "type": "go",
            "mode": "remote",
            "remotePath": "",
            "port": 38697,
            "host": "127.0.0.1",
            "showLog": true
        }
    ]
}
```

## Performance Tips

### Database
- Add indexes to frequently queried columns
- Use connection pooling
- Implement query caching with Redis

### API
- Implement pagination for list endpoints
- Add request timeout
- Use middleware for request logging

### Code
- Use sync.Pool for frequently allocated objects
- Avoid goroutine leaks
- Profile with pprof

## Common Issues

### Import Issues
```bash
# Update module cache
go mod tidy
go mod download
```

### Port Already in Use
```bash
# Find process using port
lsof -i :8080

# Kill process
kill -9 <PID>
```

### Database Connection Error
```bash
# Check PostgreSQL is running
psql -U postgres -c "SELECT 1;"

# Check database exists
psql -U postgres -l | grep wisdom_house_dev
```

## Resources

- [Go Best Practices](https://golang.org/doc/effective_go)
- [Gin Documentation](https://github.com/gin-gonic/gin)
- [GORM Documentation](https://gorm.io/docs/index.html)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
