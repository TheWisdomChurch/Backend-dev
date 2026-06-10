# Wisdom House Backend API

Professional REST API for Wisdom House Church built with Go, Gin, PostgreSQL, and Redis.

## Quick Start

```bash
# Install dependencies
go mod download

# Run development server
go run cmd/api/main.go

# Build binary
make build

# Run tests
make test
```

API available at: `http://localhost:8080/api/v1`

## Stack

- **Language**: Go 1.25+
- **Framework**: Gin Web Framework
- **Database**: PostgreSQL 15
- **Cache**: Redis 7
- **Documentation**: Swagger/OpenAPI
- **Deployment**: Docker

## Project Structure

```
├── cmd/api/                    # Application entry point
├── internal/
│   ├── apperror/              # Error handling
│   ├── authutil/              # Authentication utilities
│   ├── cache/                 # Redis caching layer
│   ├── config/                # Configuration management
│   ├── database/              # Database connections & migrations
│   ├── email/                 # Email service
│   ├── exportpdf/             # PDF export utilities
│   ├── handlers/              # HTTP request handlers
│   ├── logger/                # Logging utilities
│   ├── middleware/            # HTTP middleware
│   ├── models/                # Data models
│   ├── repository/            # Data access layer
│   ├── sanitize/              # Input sanitization
│   ├── service/               # Business logic
│   ├── validation/            # Input validation
│   └── worker/                # Background jobs
├── pkg/utils/                 # Shared utilities
├── migrations/                # Database migrations
├── dockerfile                 # Docker image
├── go.mod & go.sum           # Dependencies
└── Makefile                   # Build automation
```

## API Documentation

Swagger documentation available at: `http://localhost:8080/swagger/index.html`

## Configuration

See `.env.example` for all available environment variables.

Development: `.env`
Production: `.env.production`

## Key Endpoints

### Public
- `GET /api/v1/events` - List events
- `GET /api/v1/leadership` - List leadership
- `POST /api/v1/testimonials` - Submit testimonial
- `POST /api/v1/workforce/apply` - Apply for workforce

### Admin (Protected)
- `GET /api/v1/admin/forms` - Manage forms
- `GET /api/v1/admin/members` - Manage members
- `GET /api/v1/admin/events` - Manage events

## Development

```bash
# Watch for changes and rebuild
air

# Run tests
go test ./...

# Run specific test
go test ./internal/service -v

# Coverage report
go test -cover ./...
```

## Production

```bash
# Build optimized binary
make build-prod

# Docker build
docker build -t wisdom-house-backend .

# Docker run
docker run -p 8080:8080 --env-file .env.production wisdom-house-backend
```

## Database

```bash
# Run migrations
go run cmd/api/main.go migrate

# Seed test data
go run cmd/api/main.go seed
```

## Contributing

1. Create feature branch: `git checkout -b feature/name`
2. Commit changes: `git commit -am 'Add feature'`
3. Push to branch: `git push origin feature/name`
4. Create Pull Request

## License

See LICENSE file.
