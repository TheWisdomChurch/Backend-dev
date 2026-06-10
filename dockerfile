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
# Development
# =========================
FROM golang:1.25-alpine AS development

WORKDIR /app
RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./
RUN go mod download
COPY . .

EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

# # =========================
# # Production
# # =========================
# FROM alpine:latest AS production

# RUN apk --no-cache add ca-certificates
# WORKDIR /root/

# COPY --from=builder /app/wisdom-house .
# EXPOSE 8080

# CMD ["./wisdom-house"]
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
