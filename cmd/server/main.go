package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"github.com/hungpv1995/golang_training_2025/internal/cache"
	"github.com/hungpv1995/golang_training_2025/internal/handlers"
	"github.com/hungpv1995/golang_training_2025/internal/repository"
	"github.com/hungpv1995/golang_training_2025/internal/search"
)

func main() {
	// Initialize database
	db, err := initDB()
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Initialize Redis
	redisClient, err := initRedis()
	if err != nil {
		log.Fatal("Failed to initialize Redis:", err)
	}
	defer redisClient.Close()

	// Initialize Elasticsearch
	esClient, err := initElasticsearch()
	if err != nil {
		log.Fatal("Failed to initialize Elasticsearch:", err)
	}

	// Initialize repositories and services
	postRepo := repository.NewPostRepository(db)
	cacheService := cache.NewRedisCache(redisClient)
	searchService := search.NewElasticSearch(esClient)

	// Wait for Elasticsearch to be ready and create index
	time.Sleep(5 * time.Second)
	if err := searchService.CreateIndex(); err != nil {
		log.Printf("Failed to create Elasticsearch index: %v", err)
	}

	// Initialize handlers
	postHandler := handlers.NewPostHandler(postRepo, cacheService, searchService)

	// Setup routes
	r := mux.NewRouter()
	r.HandleFunc("/posts", postHandler.CreatePost).Methods("POST")
	r.HandleFunc("/posts/{id}", postHandler.GetPost).Methods("GET")
	r.HandleFunc("/posts/{id}", postHandler.UpdatePost).Methods("PUT")
	r.HandleFunc("/posts/search-by-tag", postHandler.SearchByTag).Methods("GET")
	r.HandleFunc("/posts/search", postHandler.SearchPosts).Methods("GET")

	// Health check endpoint
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func initDB() (*sql.DB, error) {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "bloguser")
	dbPassword := getEnv("DB_PASSWORD", "blogpass")
	dbName := getEnv("DB_NAME", "blogdb")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func initRedis() (*redis.Client, error) {
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")

	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		Password:     "", // no password set
		DB:           0,  // use default DB
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// Test connection
	ctx := client.Context()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return client, nil
}

func initElasticsearch() (*elasticsearch.Client, error) {
	esURL := getEnv("ELASTICSEARCH_URL", "http://localhost:9200")

	cfg := elasticsearch.Config{
		Addresses: []string{esURL},
		// Retry on 429 Too Many Requests
		RetryOnStatus: []int{502, 503, 504, 429},
		RetryBackoff:  func(i int) time.Duration { return time.Duration(i) * 100 * time.Millisecond },
		MaxRetries:    3,
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	// Test connection
	res, err := client.Info()
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch connection error: %s", res.String())
	}

	return client, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
