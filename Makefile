# Makefile for Go Multi-Tenant System

.PHONY: help build up down logs clean test lint

# Default target
help:
	@echo "Available commands:"
	@echo "  build     - Build all Docker images"
	@echo "  up        - Start all services"
	@echo "  down      - Stop all services"
	@echo "  logs      - Show logs for all services"
	@echo "  clean     - Remove all containers and volumes"
	@echo "  test      - Run tests"
	@echo "  lint      - Run linter"
	@echo "  dev       - Start development environment"
	@echo "  prod      - Start production environment"

# Build all Docker images
build:
	docker-compose build

# Start all services
up:
	docker-compose up -d

# Stop all services
down:
	docker-compose down

# Show logs for all services
logs:
	docker-compose logs -f

# Show logs for specific service
logs-auth:
	docker-compose logs -f auth-service

logs-tenant:
	docker-compose logs -f tenant-service

logs-location:
	docker-compose logs -f location-service

logs-streaming:
	docker-compose logs -f streaming-service

logs-gateway:
	docker-compose logs -f api-gateway

# Remove all containers and volumes
clean:
	docker-compose down -v --remove-orphans
	docker system prune -f

# Run tests
test:
	go test ./...

# Run linter
lint:
	golangci-lint run

# Start development environment
dev:
	docker-compose up -d

# Start production environment
prod:
	docker-compose up -d

# Database operations
db-migrate:
	docker-compose exec postgres psql -U postgres -d multi_tenant_db -f /docker-entrypoint-initdb.d/init.sql

db-shell:
	docker-compose exec postgres psql -U postgres -d multi_tenant_db

# Service-specific commands
restart-auth:
	docker-compose restart auth-service

restart-tenant:
	docker-compose restart tenant-service

restart-location:
	docker-compose restart location-service

restart-streaming:
	docker-compose restart streaming-service

restart-gateway:
	docker-compose restart api-gateway

# Health checks
health:
	@echo "Checking service health..."
	@curl -s http://localhost:8080/health || echo "API Gateway: DOWN"
	@curl -s http://localhost:8001/health || echo "Auth Service: DOWN"
	@curl -s http://localhost:8002/health || echo "Tenant Service: DOWN"
	@curl -s http://localhost:8003/health || echo "Location Service: DOWN"
	@curl -s http://localhost:8004/health || echo "Streaming Service: DOWN"

# Setup development environment
setup:
	@echo "Setting up development environment..."
	@cp env.example .env
	@echo "Please edit .env file with your configuration"
	@echo "Then run 'make up' to start the services"
