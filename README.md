# Blog API System

A high-performance blog API system built with Golang, featuring PostgreSQL for data storage, Redis for caching, and Elasticsearch for full-text search.

## 🚀 Quick Start

### Prerequisites
- Docker and Docker Compose installed
- Go 1.21+ (for local development)
- Make (optional, for using Makefile commands)

### Running the Application

1. Clone the repository:
```bash
git clone <repository-url>
cd blog-api
```

2. Start all services using Docker Compose:
```bash
docker-compose up -d
```

This will start:
- PostgreSQL (port 5432)
- Redis (port 6379)
- Elasticsearch (port 9200)
- Blog API Service (port 8080)

3. Wait for services to be ready (especially Elasticsearch):
```bash
# Check if Elasticsearch is ready
curl -X GET "localhost:9200/_cluster/health?wait_for_status=yellow&timeout=50s"
```

4. The API will be available at `http://localhost:8080`

## 📚 API Documentation

### 1. Create a Post
**Endpoint:** `POST /posts`

Creates a new blog post with transaction support for activity logging.

```bash
curl -X POST http://localhost:8080/posts \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Getting Started with Go",
    "content": "Go is a statically typed, compiled programming language...",
    "tags": ["golang", "programming", "backend"]
  }'
```

**Response:**
```json
{
  "id": 1,
  "title": "Getting Started with Go",
  "content": "Go is a statically typed, compiled programming language...",
  "tags": ["golang", "programming", "backend"],
  "created_at": "2024-03-15T10:00:00Z"
}
```

### 2. Get Post by ID (with Cache)
**Endpoint:** `GET /posts/:id`

Retrieves a post by ID using cache-aside pattern.

```bash
curl -X GET http://localhost:8080/posts/1
```

**Response:**
```json
{
  "id": 1,
  "title": "Getting Started with Go",
  "content": "Go is a statically typed, compiled programming language...",
  "tags": ["golang", "programming", "backend"],
  "created_at": "2024-03-15T10:00:00Z",
  "related_posts": [
    {
      "id": 2,
      "title": "Advanced Go Patterns",
      "tags": ["golang", "patterns"]
    }
  ]
}
```

### 3. Update a Post
**Endpoint:** `PUT /posts/:id`

Updates a post and invalidates the cache.

```bash
curl -X PUT http://localhost:8080/posts/1 \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Getting Started with Go - Updated",
    "content": "Updated content here...",
    "tags": ["golang", "tutorial", "backend"]
  }'
```

### 4. Search Posts by Tag
**Endpoint:** `GET /posts/search-by-tag?tag=<tag_name>`

Searches posts containing a specific tag using GIN index.

```bash
curl -X GET "http://localhost:8080/posts/search-by-tag?tag=golang"
```

**Response:**
```json
{
  "posts": [
    {
      "id": 1,
      "title": "Getting Started with Go",
      "tags": ["golang", "programming", "backend"]
    },
    {
      "id": 3,
      "title": "Go Concurrency Patterns",
      "tags": ["golang", "concurrency"]
    }
  ],
  "total": 2
}
```

### 5. Full-text Search
**Endpoint:** `GET /posts/search?q=<query>`

Performs full-text search across title and content using Elasticsearch.

```bash
curl -X GET "http://localhost:8080/posts/search?q=programming"
```

**Response:**
```json
{
  "posts": [
    {
      "id": 1,
      "title": "Getting Started with Go",
      "content": "Go is a statically typed, compiled programming language...",
      "score": 2.5
    }
  ],
  "total": 1
}
```

## 🗄️ Database Schema

### Posts Table
```sql
CREATE TABLE posts (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW()
);

-- GIN index for tag search optimization
CREATE INDEX idx_posts_tags ON posts USING GIN (tags);
```

### Activity Logs Table
```sql
CREATE TABLE activity_logs (
    id SERIAL PRIMARY KEY,
    action VARCHAR(50) NOT NULL,
    post_id INTEGER REFERENCES posts(id),
    logged_at TIMESTAMP DEFAULT NOW()
);
```

## 🔧 Technical Features

### PostgreSQL Optimizations
- **GIN Index**: Optimizes array searches for tags
- **Transactions**: Ensures data integrity between posts and activity_logs

### Redis Caching Strategy
- **Cache-Aside Pattern**: Reduces database load for frequently accessed posts
- **TTL**: 5-minute expiration for cached posts
- **Cache Invalidation**: Automatic cache clearing on updates

### Elasticsearch Integration
- **Full-text Search**: Searches across title and content fields
- **Real-time Indexing**: Automatic synchronization on create/update
- **Related Posts**: Finds similar posts based on tags (Bonus feature)

## 🧪 Testing

### Sample Data
You can populate the database with sample data:

```bash
# Create multiple posts
for i in {1..10}; do
  curl -X POST http://localhost:8080/posts \
    -H "Content-Type: application/json" \
    -d "{
      \"title\": \"Post $i\",
      \"content\": \"This is the content for post $i about programming and technology.\",
      \"tags\": [\"tag$i\", \"programming\", \"technology\"]
    }"
done
```

### Verify Features

1. **Test Cache Hit/Miss**:
```bash
# First call - cache miss (check logs)
curl -X GET http://localhost:8080/posts/1

# Second call - cache hit (should be faster)
curl -X GET http://localhost:8080/posts/1
```

2. **Test Cache Invalidation**:
```bash
# Update post
curl -X PUT http://localhost:8080/posts/1 \
  -H "Content-Type: application/json" \
  -d '{"title": "Updated Title", "content": "Updated content", "tags": ["new-tag"]}'

# Get post (should show updated data)
curl -X GET http://localhost:8080/posts/1
```

3. **Test Transaction Rollback**:
```bash
# Try creating a post with invalid data to trigger rollback
curl -X POST http://localhost:8080/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "", "content": "", "tags": []}'
```

## 📁 Project Structure

```
blog-api/
├── cmd/
│   └── server/
│       └── main.go          # Application entry point
├── internal/
│   ├── handlers/            # HTTP handlers
│   │   └── post_handler.go
│   ├── models/              # Data models
│   │   └── post.go
│   ├── repository/          # Database operations
│   │   └── post_repository.go
│   ├── cache/               # Redis cache operations
│   │   └── redis_cache.go
│   └── search/              # Elasticsearch operations
│       └── elastic_search.go
├── migrations/              # Database migrations
│   └── 001_init.sql
├── docker-compose.yml       # Docker services configuration
├── Dockerfile              # Application container
├── go.mod                  # Go dependencies
├── go.sum
└── README.md

```

## 🔍 Monitoring

### Check Service Health

```bash
# PostgreSQL
docker exec -it blog-postgres psql -U bloguser -d blogdb -c "SELECT 1"

# Redis
docker exec -it blog-redis redis-cli ping

# Elasticsearch
curl -X GET "localhost:9200/_cat/health?v"

# Check Elasticsearch indices
curl -X GET "localhost:9200/_cat/indices?v"
```

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f api
```

## 🛠️ Development

### Local Development

1. Install dependencies:
```bash
go mod download
```

2. Run migrations:
```bash
docker exec -i blog-postgres psql -U bloguser -d blogdb < migrations/001_init.sql
```

3. Run the application:
```bash
go run cmd/server/main.go
```

### Environment Variables

The application uses the following environment variables (set in docker-compose.yml):

- `DB_HOST`: PostgreSQL host
- `DB_PORT`: PostgreSQL port
- `DB_USER`: Database user
- `DB_PASSWORD`: Database password
- `DB_NAME`: Database name
- `REDIS_ADDR`: Redis address
- `ELASTICSEARCH_URL`: Elasticsearch URL

## 📝 Notes

- The system implements all required features plus the bonus "Related Posts" feature
- Cache TTL is set to 5 minutes as specified
- All database operations use proper error handling and transactions
- The GIN index significantly improves tag search performance
- Elasticsearch indexing happens asynchronously to avoid blocking the main request

## 🚧 Troubleshooting

### Elasticsearch not starting
- Increase Docker memory allocation (Elasticsearch requires at least 2GB)
- Check if port 9200 is already in use

### Database connection issues
- Ensure PostgreSQL is fully started before the API service
- Check credentials in docker-compose.yml

### Redis connection issues
- Verify Redis is running: `docker ps`
- Check Redis logs: `docker-compose logs redis`
