package models

import (
	"time"
)

// Post represents a blog post
type Post struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	RelatedPosts []Related `json:"related_posts,omitempty"`
}

// Related represents a related post
type Related struct {
	ID    int      `json:"id"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
}

// CreatePostRequest represents the request body for creating a post
type CreatePostRequest struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// UpdatePostRequest represents the request body for updating a post
type UpdatePostRequest struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// SearchResponse represents search results
type SearchResponse struct {
	Posts []interface{} `json:"posts"`
	Total int           `json:"total"`
}
