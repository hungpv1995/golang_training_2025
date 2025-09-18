.PHONY: help build up down restart logs clean test

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build Docker images
	docker-compose build

up: ## Start all services
	docker-compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 10
	@echo "Services are starting up. Check status with 'make logs'"

down: ## Stop all services
	docker-compose down

restart: ## Restart all services
	docker-compose restart

logs: ## View logs from all services
	docker-compose logs -f

logs-api: ## View logs from API service only
	docker-compose logs -f api

clean: ## Stop services and remove volumes
	docker-compose down -v
	rm -rf postgres_data redis_data elasticsearch_data

test-create: ## Test create post endpoint
	@echo "Creating a new post..."
	@curl -X POST http://localhost:8080/posts \
		-H "Content-Type: application/json" \
		-d '{"title": "Test Post", "content": "This is a test post content.", "tags": ["test", "demo"]}' \
		| jq .

test-get: ## Test get post endpoint (uses ID 1)
	@echo "Getting post with ID 1..."
	@curl -X GET http://localhost:8080/posts/1 | jq .

test-update: ## Test update post endpoint (uses ID 1)
	@echo "Updating post with ID 1..."
	@curl -X PUT http://localhost:8080/posts/1 \
		-H "Content-Type: application/json" \
		-d '{"title": "Updated Post", "content": "This is updated content.", "tags": ["updated", "test"]}' \
		| jq .

test-search-tag: ## Test search by tag endpoint
	@echo "Searching posts with tag 'golang'..."
	@curl -X GET "http://localhost:8080/posts/search-by-tag?tag=golang" | jq .

test-search: ## Test full-text search endpoint
	@echo "Searching posts with query 'programming'..."
	@curl -X GET "http://localhost:8080/posts/search?q=programming" | jq .

test-all: ## Run all tests
	@make test-create
	@echo ""
	@sleep 2
	@make test-get
	@echo ""
	@sleep 2
	@make test-update
	@echo ""
	@sleep 2
	@make test-search-tag
	@echo ""
	@sleep 2
	@make test-search

populate: ## Populate database with sample data
	@echo "Populating database with sample posts..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		curl -X POST http://localhost:8080/posts \
			-H "Content-Type: application/json" \
			-d "{\"title\": \"Sample Post $$i\", \"content\": \"This is the content for post $$i about programming and technology.\", \"tags\": [\"tag$$i\", \"programming\", \"technology\"]}" \
			-s > /dev/null; \
		echo "Created post $$i"; \
	done
	@echo "Database populated with 10 sample posts"

check-health: ## Check health of all services
	@echo "Checking PostgreSQL..."
	@docker exec blog-postgres psql -U bloguser -d blogdb -c "SELECT 1" > /dev/null && echo "✓ PostgreSQL is healthy" || echo "✗ PostgreSQL is not responding"
	@echo "Checking Redis..."
	@docker exec blog-redis redis-cli ping > /dev/null && echo "✓ Redis is healthy" || echo "✗ Redis is not responding"
	@echo "Checking Elasticsearch..."
	@curl -s -X GET "localhost:9200/_cluster/health" > /dev/null && echo "✓ Elasticsearch is healthy" || echo "✗ Elasticsearch is not responding"
	@echo "Checking API..."
	@curl -s -X GET "localhost:8080/posts/1" > /dev/null && echo "✓ API is healthy" || echo "✗ API is not responding"

dev: ## Start services and watch logs
	@make up
	@make logs
