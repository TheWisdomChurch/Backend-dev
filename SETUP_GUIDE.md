# Backend Setup Guide - PostgreSQL Required

## The Issue

The backend **requires PostgreSQL** - it's hardcoded in the codebase. The `.env` file must point to a PostgreSQL database, not SQLite.

## Solutions

### Option 1: Docker Compose (Recommended - Easiest)

```bash
# From project root
cd /root/Tech_projects_000/Frontend

# Start PostgreSQL + Redis + Backend
docker-compose -f docker-compose.dev.yml up -d

# Verify
docker-compose ps
curl http://localhost:8080/api/v1/health
```

**Pros**: No local database setup needed, everything containerized
**Cons**: Requires Docker

---

### Option 2: Local PostgreSQL Installation

#### Install PostgreSQL (macOS)
```bash
brew install postgresql@15
brew services start postgresql@15
```

#### Install PostgreSQL (Ubuntu/Debian)
```bash
sudo apt-get update
sudo apt-get install postgresql postgresql-contrib
sudo systemctl start postgresql
```

#### Create Development Database
```bash
# Connect to PostgreSQL
psql -U postgres

# In psql prompt, run:
CREATE DATABASE wisdom_house_dev;
CREATE USER postgres WITH PASSWORD 'postgres';
ALTER ROLE postgres WITH SUPERUSER;
\quit
```

#### Start Backend
```bash
# Terminal 1: Start PostgreSQL (if not auto-starting)
# macOS: brew services start postgresql@15
# Ubuntu: sudo systemctl start postgresql

# Terminal 2: Start Backend
make run

# Terminal 3: Verify
curl http://localhost:8080/api/v1/health
```

---

### Option 3: Docker Container (Just PostgreSQL)

```bash
# Start PostgreSQL container
docker run -d \
  --name postgres-wisdom \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=wisdom_house_dev \
  -p 5432:5432 \
  postgres:15-alpine

# Verify connection
psql -h localhost -U postgres -d wisdom_house_dev -c "SELECT 1;"

# Start backend
make run
```

---

## Fix Checklist

- [ ] PostgreSQL is running on localhost:5432
- [ ] Database `wisdom_house_dev` exists
- [ ] `.env` file has `DATABASE_URL=postgres://postgres:postgres@localhost:5432/wisdom_house_dev?sslmode=disable`
- [ ] Run `go mod tidy` (already done)
- [ ] Run `make run` successfully

---

## Verify Setup

### Check PostgreSQL Connection
```bash
psql -h localhost -U postgres -d wisdom_house_dev -c "SELECT version();"
```

### Check Backend Build
```bash
make build
./bin/wisdomhousebe --version  # if supported
```

### Check Backend Running
```bash
make run
# Should see: 🚀 ... Loading configuration...
```

### Check API Health
```bash
curl http://localhost:8080/api/v1/health
# Should return JSON with status: ok
```

---

## Common Issues & Fixes

### Issue: "dial unix /tmp/.s.PGSQL.5432: connect: no such file or directory"
**Cause**: PostgreSQL not running
**Fix**: Start PostgreSQL
```bash
# macOS
brew services start postgresql@15

# Ubuntu
sudo systemctl start postgresql

# Docker
docker start postgres-wisdom
```

### Issue: "database wisdom_house_dev does not exist"
**Cause**: Database not created
**Fix**: Create database
```bash
psql -U postgres -c "CREATE DATABASE wisdom_house_dev;"
```

### Issue: "go: command not found"
**Cause**: Go not installed
**Fix**: Install Go 1.25+
```bash
# macOS
brew install go

# Ubuntu
wget https://go.dev/dl/go1.25.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.5.linux-amd64.tar.gz
```

### Issue: "permission denied" when running `make`
**Cause**: Makefile not executable
**Fix**: Make file executable
```bash
chmod +x makefile
# Or just use: go run cmd/api/main.go
```

---

## go.mod Status

✅ **go.mod is correct** - No fixes needed

The module path is `wisdomHouse-backend` and all imports are correct.

```bash
go mod verify    # Returns: all modules verified
go mod tidy      # Already done - no changes needed
```

---

## Environment Variables Explained

| Variable | Default | Purpose |
|----------|---------|---------|
| `ENVIRONMENT` | development | Dev vs prod mode |
| `DATABASE_URL` | Required | PostgreSQL connection string |
| `REDIS_URL` | Optional | Redis cache (empty = disabled) |
| `AUTH_SECRET_KEY` | Required | JWT signing key |
| `CORS_ALLOW_ORIGIN` | localhost:3000 | Frontend URL |

---

## Next Steps

1. **Choose setup option** (Docker is easiest)
2. **Start PostgreSQL**
3. **Run `make run`**
4. **Test API** at `http://localhost:8080/api/v1/health`
5. **Start frontend** separately

---

## Production Deployment

For production, use `.env.production` with:
- Real PostgreSQL host/credentials
- Real Redis instance
- Real AUTH_SECRET_KEY
- Real domain in CORS_ALLOW_ORIGIN

```bash
# Build for production
make build-prod

# Run with production config
.env.production go run cmd/api/main.go
```

Or use Docker:
```bash
make docker-build
docker run -p 8080:8080 --env-file .env.production wisdom-house-backend:latest
```
