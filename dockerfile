# Multi-stage Dockerfile for both dev and prod
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Debugging: List files to see what's copied
RUN ls -la /app
RUN ls -la /app/cmd || echo "cmd directory not found"
RUN ls -la /app/cmd/api || echo "api directory not found"
RUN CGO_ENABLED=0 GOOS=linux go build -o wisdom-house ./cmd/api

# Development stage - includes Air for hot reload
FROM golang:1.25-alpine AS development
WORKDIR /app
RUN go install github.com/air-verse/air@latest
COPY go.mod go.sum ./
RUN go mod download
COPY . .
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

# Production stage - minimal Alpine image
FROM alpine:latest AS production
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/wisdom-house .
EXPOSE 8080
CMD ["./wisdom-house"]