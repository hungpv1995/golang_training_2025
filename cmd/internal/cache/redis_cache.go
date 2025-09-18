package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hungpv1995/golang_training_2025/internal/models"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{
		client: client,
		ctx:    context.Background(),
	}
}

// GetPost retrieves a post from cache
func (c *RedisCache) GetPost(postID int) (*models.Post, error) {
	cacheKey := fmt.Sprintf("post:%d", postID)

	data, err := c.client.Get(c.ctx, cacheKey).Result()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get from cache: %w", err)
	}

	var post models.Post
	if err := json.Unmarshal([]byte(data), &post); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}

	return &post, nil
}

// SetPost stores a post in cache with TTL
func (c *RedisCache) SetPost(post *models.Post, ttl time.Duration) error {
	cacheKey := fmt.Sprintf("post:%d", post.ID)

	data, err := json.Marshal(post)
	if err != nil {
		return fmt.Errorf("failed to marshal post: %w", err)
	}

	if err := c.client.Set(c.ctx, cacheKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// InvalidatePost removes a post from cache
func (c *RedisCache) InvalidatePost(postID int) error {
	cacheKey := fmt.Sprintf("post:%d", postID)

	if err := c.client.Del(c.ctx, cacheKey).Err(); err != nil {
		return fmt.Errorf("failed to invalidate cache: %w", err)
	}

	return nil
}

// Ping checks if Redis is available
func (c *RedisCache) Ping() error {
	return c.client.Ping(c.ctx).Err()
}
