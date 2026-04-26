# --- Builder ---
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /gix-server ./cmd/gix-server
RUN CGO_ENABLED=0 GOOS=linux go build -o /cost-estimator ./cmd/cost-estimator

# --- Production ---
FROM alpine:3.23.4

# Security: Create a non-root user
RUN addgroup -S gixgroup && adduser -S gixuser -G gixgroup

WORKDIR /home/gixuser
RUN apk --no-cache add ca-certificates

# Copy binaries from builder
COPY --from=builder /gix-server .
COPY --from=builder /cost-estimator .

# Security: Change ownership to non-root user
RUN chown -R gixuser:gixgroup /home/gixuser

# Security: Switch to non-root user
USER gixuser

EXPOSE 8080
EXPOSE 8081

# Default command, overridden in K8s for the estimator
CMD ["./gix-server"]
