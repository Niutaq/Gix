# Dockerfile
# --- Base ---
FROM golang:1.25-alpine AS base
RUN apk add --no-cache git build-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

# --- Development ---
FROM base AS dev
RUN go install github.com/air-verse/air@latest
COPY . .
CMD ["air", "-c", ".air.toml"]

# --- Production ---
FROM base AS prod
COPY . .
RUN CGO_ENABLED=0 go build -o /gix-server ./cmd/gix-server/main.go
FROM alpine:latest
WORKDIR /
COPY --from=prod /gix-server /gix-server
COPY --from=prod /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
EXPOSE 8080
CMD ["/gix-server"]
