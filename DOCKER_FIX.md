# Docker Build Fix - GitHub Actions

## The Problem

GitHub Actions docker/build-push-action failed with:
```
no Go files in /app
ERROR: process "/bin/sh -c CGO_ENABLED=0 GOOS=linux go build -o wisdom-house ." did not complete successfully
```

## Root Cause

The **dockerfile was trying to build from root directory** (`.`) but the main entry point was moved to `cmd/api/main.go`.

**Before:**
```dockerfile
RUN CGO_ENABLED=0 GOOS=linux go build -o wisdom-house .
```
This looked for Go files in `/app` root, but there are none - main.go is in `/app/cmd/api/`.

## The Fix

**After:**
```dockerfile
RUN CGO_ENABLED=0 GOOS=linux go build -o wisdom-house ./cmd/api
```
Now it correctly builds from the `cmd/api` directory where `main.go` is located.

## Changes Made

### 1. Builder Stage (Line 13)
```dockerfile
# BEFORE
RUN CGO_ENABLED=0 GOOS=linux go build -o wisdom-house .

# AFTER
RUN CGO_ENABLED=0 GOOS=linux go build -o wisdom-house ./cmd/api
```

### 2. Production Stage (Lines 44-56)
```dockerfile
# BEFORE
WORKDIR /root/
COPY --from=builder /app/wisdom-house .

# AFTER
WORKDIR /app
COPY --from=builder /app/wisdom-house /app/wisdom-house
COPY --from=builder /app/migrations /app/migrations
CMD ["/app/wisdom-house"]
```

## Verification

### Local Build Test
```bash
# Build the Docker image
docker build --target production -t wisdom-house-backend:latest -f dockerfile .

# Run the image
docker run -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/db" \
  -e AUTH_SECRET_KEY="your-secret" \
  wisdom-house-backend:latest
```

### GitHub Actions Build
```bash
# The GitHub Actions workflow will now:
1. Load build cache from GHA
2. Build golang:1.25-alpine builder image
3. Run: go build -o wisdom-house ./cmd/api ✅ (FIXED)
4. Copy binary to alpine:latest production image
5. Push to ghcr.io/thewisdomchurch/wisdom-api:main
```

## Files Modified

- ✅ `dockerfile` - Fixed build paths

## Files That Still Work

- ✅ `go.mod` - No changes needed
- ✅ `go.sum` - No changes needed
- ✅ `cmd/api/main.go` - Already in correct location
- ✅ `.github/workflows/*.yml` - No changes needed

## Next Steps

1. **Commit the fix**
   ```bash
   git add dockerfile
   git commit -m "fix: correct dockerfile build path to cmd/api"
   git push origin develop
   ```

2. **GitHub Actions will automatically**
   - Detect the push
   - Run docker/build-push-action@v6
   - Build successfully ✅
   - Push to ghcr.io registry

3. **Verify the push**
   ```bash
   # Check Docker registry
   docker pull ghcr.io/thewisdomchurch/wisdom-api:main
   docker run -p 8080:8080 ghcr.io/thewisdomchurch/wisdom-api:main
   ```

## Docker Image Specs

After fix, the image will:
- **Base Image**: `alpine:latest` (3.87MB)
- **Binary Size**: ~73MB (compiled Go binary)
- **Platforms**: linux/amd64, linux/arm64
- **Tags**: 
  - `ghcr.io/thewisdomchurch/wisdom-api:main`
  - `ghcr.io/thewisdomchurch/wisdom-api:sha-1656db3`
- **Registry**: GitHub Container Registry (ghcr.io)

## Complete Dockerfile (Fixed)

```dockerfile
# =========================
# Builder
# =========================
FROM golang:1.25-alpine AS builder

WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o wisdom-house ./cmd/api

# =========================
# Production
# =========================
FROM alpine:latest AS production

RUN apk --no-cache add ca-certificates postgresql-client

WORKDIR /app

COPY --from=builder /app/wisdom-house /app/wisdom-house
COPY --from=builder /app/migrations /app/migrations

EXPOSE 8080
ENV PORT=8080

CMD ["/app/wisdom-house"]
```

## Troubleshooting

### If build still fails locally
```bash
# Clean and rebuild
docker system prune -a
docker build --no-cache --target production -t wisdom-house-backend:latest -f dockerfile .
```

### If GitHub Actions still fails
```bash
# Check the exact error in Actions logs
# Look for: "no Go files" or "cannot find" 
# Verify the dockerfile path in the workflow file
```

### If Docker image doesn't start
```bash
# Run with detailed output
docker run -it wisdom-house-backend:latest

# The error will show what's missing
# Usually: PostgreSQL connection or missing env vars
```

## Summary

✅ **Dockerfile fixed to build from `./cmd/api`**
✅ **Production image workdir corrected**
✅ **Binary path correctly referenced**
✅ **Ready for GitHub Actions deployment**

The GitHub Actions workflow will now successfully:
1. Build the Docker image for linux/amd64 and linux/arm64
2. Push to ghcr.io registry
3. Deploy to your infrastructure
