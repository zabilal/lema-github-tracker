
.PHONY: help install-tools swagger-gen build run clean test docker-build docker-run docker-up docker-down

# Default target
help:
	@echo "Available targets:"
	@echo "  install-tools  - Install required tools (swag, etc.)"
	@echo "  swagger-gen    - Generate Swagger documentation"
	@echo "  build         - Build the application"
	@echo "  run           - Run the application"
	@echo "  clean         - Clean build artifacts"
	@echo "  test          - Run tests"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"

# Install required tools
install-tools:
	@echo "Installing swag for Swagger documentation generation..."
	go install github.com/swaggo/swag/cmd/swag@latest
	@echo "Installing air for auto-reload..."
	go install github.com/air-verse/air@latest
	@echo "Installing other dependencies..."
	go mod tidy
	go mod download

# Generate Swagger documentation
swagger-gen:
	@echo "Generating Swagger documentation..."
	swag init -g ./cmd/server/main.go -o ./docs --parseDependency --parseInternal
	@echo "Swagger documentation generated in ./docs/"

# Build the application
build: swagger-gen
	@echo "Building GitHub Service..."
	go build -o bin/github-service ./cmd/server/main.go

# Run the application
run: swagger-gen
	@echo "Starting GitHub Service..."
	@echo "Swagger UI will be available at: http://localhost:8080/swagger/index.html"
	go run ./cmd/server/main.go

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf docs/
	go clean

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Build Docker image
docker-build: swagger-gen
	@echo "Building Docker image..."
	docker build -t github-service:latest .

# Run Docker container
docker-run:
	@echo "Running Docker container..."
	@echo "Swagger UI will be available at: http://localhost:8080/swagger/index.html"
	docker run -p 8080:8080 github-service:latest

# Development server with auto-reload (requires air)
dev:
	@echo "Starting development server with auto-reload..."
	@echo "Install air first: go install github.com/air-verse/air@latest"
	air

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	golangci-lint run

# Generate and serve Swagger docs only
swagger-serve: swagger-gen
	@echo "Serving Swagger documentation..."
	@echo "Open http://localhost:8081 in your browser"
	@cd docs && python3 -m http.server 8081

# Update dependencies
deps-update:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

# Security scan (requires gosec)
security-scan:
	@echo "Running security scan..."
	gosec ./...

# Complete setup for new developers
setup: install-tools swagger-gen
	@echo "Setup completed!"
	@echo "Run 'make run' to start the application"
	@echo "Run 'make test' to run tests"
	@echo "Swagger UI will be available at: http://localhost:8080/swagger/index.html"

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f github-service

# Development setup
dev-setup:
	cp .env.example .env
	docker-compose up -d postgres redis
	sleep 5
	make run

# Database migration (if you have migration tool)
migrate-up:
	migrate -path ./migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path ./migrations -database "$(DATABASE_URL)" down

# API testing
test-api:
	@echo "Testing health endpoint..."
	curl -s http://localhost:8080/api/v1/health | jq
	@echo "\nTesting sync repository..."
	curl -s -X POST http://localhost:8080/api/v1/repositories/sync \
		-H "Content-Type: application/json" \
		-d '{"owner": "octocat", "repository": "Hello-World"}' | jq