# Dockerfile (with K8s)
# --- Builder ---
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /gix-server ./cmd/gix-server

# --- Production ---
FROM alpine:3.20.2
WORKDIR /root/
RUN apk --no-cache add ca-certificates
COPY --from=builder /gix-server .
EXPOSE 8080
EXPOSE 8081

CMD ["./gix-server"]
