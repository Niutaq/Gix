#!/bin/bash

# Gix Control Script - All-in-One
# Usage: ./gix.sh [local|remote|stop|logs|deploy]

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}=== Gix Control Center ===${NC}"

# Stop and Clean everything
stop_all() {
    echo -e "${YELLOW}Stopping all services and cleaning up...${NC}"
    docker-compose -f docker-compose.yml -f docker-compose-infra.yml down --remove-orphans 2>/dev/null
    docker network prune -f 2>/dev/null
    task clean
    # Kill anything on port 8080 and 8081 (Backend & dRPC)
    lsof -ti:8080,8081 | xargs kill -9 2>/dev/null || true
    pkill -f "gix-server|cost-estimator|Gix.app" || true
    sleep 2
}

# Run UI helper (macOS only)
run_ui() {
    local api_url=$1
    echo -e "${BLUE}Building UI...${NC}"
    task build:macos
    echo -e "${GREEN}Launching Gix.app connected to $api_url...${NC}"
    ./Gix.app/Contents/MacOS/Gix -api "$api_url"
    return 0
}

case "$1" in
    dev)
        echo -e "${YELLOW}Deep Cleaning: Removing cache and cleaning build artifacts...${NC}"
        rm -rf "$HOME/Library/Application Support/gix/cache.json" 2>/dev/null
        clear
        task clean
        $0 local
        ;;

    local)
        stop_all
        echo -e "${GREEN}Starting Infra (Docker)...${NC}"
        docker-compose -f docker-compose.yml -f docker-compose-infra.yml up -d
        
        echo -e "${YELLOW}Waiting for containers to be ready...${NC}"
        sleep 8
        
        echo -e "${YELLOW}Starting Backend Services...${NC}"
        
        if [ -f ".env" ]; then
            source .env
            export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5433/${POSTGRES_DB}?sslmode=disable"
        else
            export DATABASE_URL="postgres://gixuser:gixpassword@localhost:5433/gix_rates?sslmode=disable"
        fi
        
        export REDIS_URL="redis://localhost:6379/0?protocol=2"
        export NATS_URL="nats://localhost:4222"
        
        go run ./cmd/gix-server &
        SERVER_PID=$!
        go run ./cmd/cost-estimator &
        EST_PID=$!
        
        sleep 4
        run_ui "http://localhost:8080"
        
        kill $SERVER_PID $EST_PID 2>/dev/null || true
        stop_all
        ;;
        
    remote)
        stop_all
        echo -e "${GREEN}Starting UI connected to Cloud API...${NC}"
        run_ui "http://165.227.246.100:8080"
        ;;
        
    stop)
        stop_all
        ;;
        
    logs)
        kubectl logs -l app=gix-backend -f
        ;;
        
    deploy)
        TAG=$(date +%Y%m%d-%H%M)
        echo -e "${BLUE}Building and Pushing Backend (Tag: $TAG)...${NC}"
        docker buildx build --platform linux/amd64 -t niutaq/gix-backend:$TAG --push .
        docker buildx build --platform linux/amd64 -t niutaq/gix-backend:latest --push .
        sed "s|image: niutaq/gix-backend:.*|image: niutaq/gix-backend:$TAG|g" k8s/04-backend.yaml > k8s/04-backend-current.yaml
        kubectl apply -f k8s/01-config.yaml -f k8s/02-db.yaml -f k8s/03-redis.yaml -f k8s/06-nats.yaml -f k8s/04-backend-current.yaml -f k8s/07-cost-estimator.yaml
        ;;
        
    *)
        echo "Usage: ./gix.sh {local|remote|stop|logs|deploy}"
        echo ""
        echo "  local   - Runs infra (Docker) + Backend & App natively (Mac)"
        echo "  remote  - Runs native App connected to Cloud API"
        echo "  stop    - Stops everything and cleans up"
        exit 1
        ;;
esac
