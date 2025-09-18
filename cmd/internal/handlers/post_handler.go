package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/hungpv1995/golang_training_2025/internal/cache"
	"github.com/hungpv1995/golang_training_2025/internal/models"
	"github.com/hungpv1995/golang_training_2025/internal/repository"
	"github.com/hungpv1995/golang_training_2025/internal/search"
)

type PostHandler struct {
	repo   *repository.PostRepository
	cache  *cache.RedisCache
	search *search.ElasticSearch
}

func NewPostHandler(repo *repository.PostRepository, cache *cache.RedisCache, search *search.ElasticSearch) *PostHandler {
	return &PostHandler{
		repo:   repo,
		cache:  cache,
		search: search,
	}
}

// CreatePost handles POST /posts
func (h *PostHandler) CreatePost(w http.ResponseWriter, r *http.Request) {
	var req models.CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Title == "" || req.Content == "" {
		http.Error(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	// Create post with transaction
	post, err := h.repo.CreatePostWithTransaction(&req)
	if err != nil {
		log.Printf("Failed to create post: %v", err)
		http.Error(w, "Failed to create post", http.StatusInternalServerError)
		return
	}

	// Index in Elasticsearch asynchronously
	go func() {
		if err := h.search.IndexPost(post); err != nil {
			log.Printf("Failed to index post in Elasticsearch: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(post)
}

// GetPost handles GET /posts/:id
func (h *PostHandler) GetPost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid post ID", http.StatusBadRequest)
		return
	}

	// Try to get from cache first
	cachedPost, err := h.cache.GetPost(id)
	if err != nil {
		log.Printf("Cache error: %v", err)
	}

	if cachedPost != nil {
		// Cache hit
		log.Printf("Cache hit for post %d", id)
		// Add related posts
		cachedPost.RelatedPosts = h.search.GetRelatedPosts(cachedPost.ID, cachedPost.Tags)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cachedPost)
		return
	}

	// Cache miss - get from database
	log.Printf("Cache miss for post %d", id)
	post, err := h.repo.GetPostByID(id)
	if err != nil {
		if err.Error() == "post not found" {
			http.Error(w, "Post not found", http.StatusNotFound)
		} else {
			log.Printf("Failed to get post: %v", err)
			http.Error(w, "Failed to get post", http.StatusInternalServerError)
		}
		return
	}

	// Cache the result with 5 minute TTL
	if err := h.cache.SetPost(post, 5*time.Minute); err != nil {
		log.Printf("Failed to cache post: %v", err)
	}

	// Get related posts (bonus feature)
	post.RelatedPosts = h.search.GetRelatedPosts(post.ID, post.Tags)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// UpdatePost handles PUT /posts/:id
func (h *PostHandler) UpdatePost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid post ID", http.StatusBadRequest)
		return
	}

	var req models.UpdatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Title == "" || req.Content == "" {
		http.Error(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	// Update in database
	if err := h.repo.UpdatePost(id, &req); err != nil {
		if err.Error() == "post not found" {
			http.Error(w, "Post not found", http.StatusNotFound)
		} else {
			log.Printf("Failed to update post: %v", err)
			http.Error(w, "Failed to update post", http.StatusInternalServerError)
		}
		return
	}

	// Invalidate cache
	if err := h.cache.InvalidatePost(id); err != nil {
		log.Printf("Failed to invalidate cache: %v", err)
	}
	log.Printf("Cache invalidated for post %d", id)

	// Update in Elasticsearch asynchronously
	go func() {
		// Get the updated post to index it
		post, err := h.repo.GetPostByID(id)
		if err != nil {
			log.Printf("Failed to get post for indexing: %v", err)
			return
		}
		if err := h.search.IndexPost(post); err != nil {
			log.Printf("Failed to update post in Elasticsearch: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Post updated successfully",
	})
}

// SearchByTag handles GET /posts/search-by-tag?tag=<tag_name>
func (h *PostHandler) SearchByTag(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		http.Error(w, "Tag parameter is required", http.StatusBadRequest)
		return
	}

	posts, err := h.repo.SearchPostsByTag(tag)
	if err != nil {
		log.Printf("Failed to search posts by tag: %v", err)
		http.Error(w, "Failed to search posts", http.StatusInternalServerError)
		return
	}

	response := models.SearchResponse{
		Posts: make([]interface{}, len(posts)),
		Total: len(posts),
	}
	for i, post := range posts {
		response.Posts[i] = post
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SearchPosts handles GET /posts/search?q=<query>
func (h *PostHandler) SearchPosts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter is required", http.StatusBadRequest)
		return
	}

	posts, err := h.search.SearchPosts(query)
	if err != nil {
		log.Printf("Failed to search posts: %v", err)
		http.Error(w, "Failed to search posts", http.StatusInternalServerError)
		return
	}

	response := models.SearchResponse{
		Posts: make([]interface{}, len(posts)),
		Total: len(posts),
	}
	for i, post := range posts {
		response.Posts[i] = post
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
