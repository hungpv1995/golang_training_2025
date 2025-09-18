package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// Post model
type Post struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	RelatedPosts []Related `json:"related_posts,omitempty"`
}

type Related struct {
	ID    int      `json:"id"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
}

// Global variables
var (
	db          *sql.DB
	redisClient *redis.Client
	esClient    *elasticsearch.Client
	ctx         = context.Background()
)

func main() {
	// Initialize database
	if err := initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Initialize Redis
	if err := initRedis(); err != nil {
		log.Fatal("Failed to initialize Redis:", err)
	}
	defer redisClient.Close()

	// Initialize Elasticsearch
	if err := initElasticsearch(); err != nil {
		log.Fatal("Failed to initialize Elasticsearch:", err)
	}

	// Wait for Elasticsearch to be ready and create index
	time.Sleep(5 * time.Second)
	createElasticsearchIndex()

	// Setup routes
	r := mux.NewRouter()
	r.HandleFunc("/posts", createPost).Methods("POST")
	r.HandleFunc("/posts/{id}", getPost).Methods("GET")
	r.HandleFunc("/posts/{id}", updatePost).Methods("PUT")
	r.HandleFunc("/posts/search-by-tag", searchByTag).Methods("GET")
	r.HandleFunc("/posts/search", searchPosts).Methods("GET")

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func initDB() error {
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "bloguser"
	}
	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		dbPassword = "blogpass"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "blogdb"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db.Ping()
}

func initRedis() error {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})

	return redisClient.Ping(ctx).Err()
}

func initElasticsearch() error {
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}

	cfg := elasticsearch.Config{
		Addresses: []string{esURL},
	}

	var err error
	esClient, err = elasticsearch.NewClient(cfg)
	if err != nil {
		return err
	}

	res, err := esClient.Info()
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func createElasticsearchIndex() {
	mapping := `{
		"mappings": {
			"properties": {
				"id": {"type": "integer"},
				"title": {"type": "text"},
				"content": {"type": "text"},
				"tags": {"type": "keyword"},
				"created_at": {"type": "date"}
			}
		}
	}`

	req := esapi.IndicesCreateRequest{
		Index: "posts",
		Body:  strings.NewReader(mapping),
	}

	res, err := req.Do(ctx, esClient)
	if err != nil {
		log.Printf("Error creating index: %s", err)
		return
	}
	defer res.Body.Close()

	if res.IsError() && !strings.Contains(res.String(), "resource_already_exists_exception") {
		log.Printf("Error creating index: %s", res.String())
	}
}

// POST /posts - Create new post with transaction
func createPost(w http.ResponseWriter, r *http.Request) {
	var post Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate input
	if post.Title == "" || post.Content == "" {
		http.Error(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insert post
	var postID int
	err = tx.QueryRow(
		`INSERT INTO posts (title, content, tags)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		post.Title, post.Content, post.Tags,
	).Scan(&postID, &post.CreatedAt)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	post.ID = postID

	// Insert activity log
	_, err = tx.Exec(
		`INSERT INTO activity_logs (action, post_id) VALUES ($1, $2)`,
		"new_post", postID,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Index in Elasticsearch
	go indexPostToElasticsearch(post)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// GET /posts/:id - Get post with cache-aside pattern
func getPost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	cacheKey := fmt.Sprintf("post:%d", id)

	// Try to get from cache
	cachedData, err := redisClient.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit
		log.Printf("Cache hit for post %d", id)
		var post Post
		if err := json.Unmarshal([]byte(cachedData), &post); err == nil {
			// Get related posts
			post.RelatedPosts = getRelatedPosts(post.ID, post.Tags)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(post)
			return
		}
	}

	// Cache miss - get from database
	log.Printf("Cache miss for post %d", id)
	var post Post
	var tagsArray sql.NullString

	err = db.QueryRow(
		`SELECT id, title, content, array_to_string(tags, ','), created_at
		 FROM posts WHERE id = $1`,
		id,
	).Scan(&post.ID, &post.Title, &post.Content, &tagsArray, &post.CreatedAt)

	if err == sql.ErrNoRows {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse tags
	if tagsArray.Valid && tagsArray.String != "" {
		post.Tags = strings.Split(tagsArray.String, ",")
	} else {
		post.Tags = []string{}
	}

	// Cache the result with 5 minute TTL
	postJSON, _ := json.Marshal(post)
	redisClient.Set(ctx, cacheKey, postJSON, 5*time.Minute)

	// Get related posts (bonus feature)
	post.RelatedPosts = getRelatedPosts(post.ID, post.Tags)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// PUT /posts/:id - Update post and invalidate cache
func updatePost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var post Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update in database
	_, err = db.Exec(
		`UPDATE posts SET title = $1, content = $2, tags = $3 WHERE id = $4`,
		post.Title, post.Content, post.Tags, id,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("post:%d", id)
	redisClient.Del(ctx, cacheKey)
	log.Printf("Cache invalidated for post %d", id)

	// Update in Elasticsearch
	post.ID = id
	go indexPostToElasticsearch(post)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Post updated successfully"})
}

// GET /posts/search-by-tag?tag=<tag_name> - Search by tag using GIN index
func searchByTag(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		http.Error(w, "Tag parameter is required", http.StatusBadRequest)
		return
	}

	rows, err := db.Query(
		`SELECT id, title, tags FROM posts WHERE $1 = ANY(tags)`,
		tag,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var posts []map[string]interface{}
	for rows.Next() {
		var id int
		var title string
		var tagsArray sql.NullString

		if err := rows.Scan(&id, &title, &tagsArray); err != nil {
			continue
		}

		tags := []string{}
		if tagsArray.Valid && tagsArray.String != "" {
			tags = strings.Split(tagsArray.String, ",")
		}

		posts = append(posts, map[string]interface{}{
			"id":    id,
			"title": title,
			"tags":  tags,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"posts": posts,
		"total": len(posts),
	})
}

// GET /posts/search?q=<query> - Full-text search using Elasticsearch
func searchPosts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter is required", http.StatusBadRequest)
		return
	}

	// Build the search query
	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"title", "content"},
			},
		},
	}

	searchJSON, _ := json.Marshal(searchQuery)

	// Perform search
	res, err := esClient.Search(
		esClient.Search.WithContext(ctx),
		esClient.Search.WithIndex("posts"),
		esClient.Search.WithBody(strings.NewReader(string(searchJSON))),
		esClient.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		http.Error(w, res.String(), http.StatusInternalServerError)
		return
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract hits
	var posts []map[string]interface{}
	if hits, ok := result["hits"].(map[string]interface{}); ok {
		if hitsArray, ok := hits["hits"].([]interface{}); ok {
			for _, hit := range hitsArray {
				if hitMap, ok := hit.(map[string]interface{}); ok {
					source := hitMap["_source"].(map[string]interface{})
					score := hitMap["_score"].(float64)
					source["score"] = score
					posts = append(posts, source)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"posts": posts,
		"total": len(posts),
	})
}

// Helper function to index post to Elasticsearch
func indexPostToElasticsearch(post Post) {
	docID := strconv.Itoa(post.ID)

	doc := map[string]interface{}{
		"id":         post.ID,
		"title":      post.Title,
		"content":    post.Content,
		"tags":       post.Tags,
		"created_at": post.CreatedAt,
	}

	docJSON, _ := json.Marshal(doc)

	req := esapi.IndexRequest{
		Index:      "posts",
		DocumentID: docID,
		Body:       strings.NewReader(string(docJSON)),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, esClient)
	if err != nil {
		log.Printf("Error indexing document: %s", err)
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Printf("Error indexing document: %s", res.String())
	} else {
		log.Printf("Document indexed successfully: %s", docID)
	}
}

// Bonus: Get related posts based on tags
func getRelatedPosts(currentPostID int, tags []string) []Related {
	if len(tags) == 0 {
		return []Related{}
	}

	// Build Elasticsearch query for related posts
	shouldClauses := []map[string]interface{}{}
	for _, tag := range tags {
		shouldClauses = append(shouldClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"tags": tag,
			},
		})
	}

	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"should": shouldClauses,
				"must_not": map[string]interface{}{
					"term": map[string]interface{}{
						"id": currentPostID,
					},
				},
				"minimum_should_match": 1,
			},
		},
		"size": 5,
	}

	searchJSON, _ := json.Marshal(searchQuery)

	res, err := esClient.Search(
		esClient.Search.WithContext(ctx),
		esClient.Search.WithIndex("posts"),
		esClient.Search.WithBody(strings.NewReader(string(searchJSON))),
	)
	if err != nil {
		log.Printf("Error searching related posts: %s", err)
		return []Related{}
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Printf("Error searching related posts: %s", res.String())
		return []Related{}
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return []Related{}
	}

	var relatedPosts []Related
	if hits, ok := result["hits"].(map[string]interface{}); ok {
		if hitsArray, ok := hits["hits"].([]interface{}); ok {
			for _, hit := range hitsArray {
				if hitMap, ok := hit.(map[string]interface{}); ok {
					source := hitMap["_source"].(map[string]interface{})

					related := Related{
						ID:    int(source["id"].(float64)),
						Title: source["title"].(string),
					}

					if tagsInterface, ok := source["tags"].([]interface{}); ok {
						for _, t := range tagsInterface {
							if tagStr, ok := t.(string); ok {
								related.Tags = append(related.Tags, tagStr)
							}
						}
					}

					relatedPosts = append(relatedPosts, related)
				}
			}
		}
	}

	return relatedPosts
}
