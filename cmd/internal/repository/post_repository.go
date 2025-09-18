package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hungpv1995/golang_training_2025/internal/models"
	_ "github.com/lib/pq"
)

type PostRepository struct {
	db *sql.DB
}

func NewPostRepository(db *sql.DB) *PostRepository {
	return &PostRepository{db: db}
}

// CreatePostWithTransaction creates a new post and logs the activity in a transaction
func (r *PostRepository) CreatePostWithTransaction(post *models.CreatePostRequest) (*models.Post, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert post
	var newPost models.Post
	err = tx.QueryRow(
		`INSERT INTO posts (title, content, tags)
		 VALUES ($1, $2, $3)
		 RETURNING id, title, content, tags, created_at`,
		post.Title, post.Content, post.Tags,
	).Scan(&newPost.ID, &newPost.Title, &newPost.Content, &newPost.Tags, &newPost.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to insert post: %w", err)
	}

	// Insert activity log
	_, err = tx.Exec(
		`INSERT INTO activity_logs (action, post_id) VALUES ($1, $2)`,
		"new_post", newPost.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert activity log: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &newPost, nil
}

// GetPostByID retrieves a post by its ID
func (r *PostRepository) GetPostByID(id int) (*models.Post, error) {
	var post models.Post
	var tagsArray sql.NullString

	err := r.db.QueryRow(
		`SELECT id, title, content, array_to_string(tags, ','), created_at
		 FROM posts WHERE id = $1`,
		id,
	).Scan(&post.ID, &post.Title, &post.Content, &tagsArray, &post.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get post: %w", err)
	}

	// Parse tags
	if tagsArray.Valid && tagsArray.String != "" {
		post.Tags = strings.Split(tagsArray.String, ",")
	} else {
		post.Tags = []string{}
	}

	return &post, nil
}

// UpdatePost updates an existing post
func (r *PostRepository) UpdatePost(id int, post *models.UpdatePostRequest) error {
	result, err := r.db.Exec(
		`UPDATE posts SET title = $1, content = $2, tags = $3 WHERE id = $4`,
		post.Title, post.Content, post.Tags, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update post: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("post not found")
	}

	return nil
}

// SearchPostsByTag searches posts by a specific tag using GIN index
func (r *PostRepository) SearchPostsByTag(tag string) ([]map[string]interface{}, error) {
	rows, err := r.db.Query(
		`SELECT id, title, tags FROM posts WHERE $1 = ANY(tags)`,
		tag,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search posts: %w", err)
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
			// PostgreSQL returns array as string, need proper parsing
			tagsStr := strings.Trim(tagsArray.String, "{}")
			if tagsStr != "" {
				// Split by comma and clean up quotes
				parts := strings.Split(tagsStr, ",")
				for _, part := range parts {
					tag := strings.Trim(part, "\"")
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			}
		}

		posts = append(posts, map[string]interface{}{
			"id":    id,
			"title": title,
			"tags":  tags,
		})
	}

	return posts, nil
}
