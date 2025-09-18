package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/hungpv1995/golang_training_2025/internal/models"
)

type ElasticSearch struct {
	client *elasticsearch.Client
	ctx    context.Context
}

func NewElasticSearch(client *elasticsearch.Client) *ElasticSearch {
	return &ElasticSearch{
		client: client,
		ctx:    context.Background(),
	}
}

// CreateIndex creates the posts index with proper mapping
func (es *ElasticSearch) CreateIndex() error {
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

	res, err := req.Do(es.ctx, es.client)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() && !strings.Contains(res.String(), "resource_already_exists_exception") {
		return fmt.Errorf("error creating index: %s", res.String())
	}

	return nil
}

// IndexPost indexes a post in Elasticsearch
func (es *ElasticSearch) IndexPost(post *models.Post) error {
	docID := strconv.Itoa(post.ID)

	doc := map[string]interface{}{
		"id":         post.ID,
		"title":      post.Title,
		"content":    post.Content,
		"tags":       post.Tags,
		"created_at": post.CreatedAt,
	}

	docJSON, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      "posts",
		DocumentID: docID,
		Body:       bytes.NewReader(docJSON),
		Refresh:    "true",
	}

	res, err := req.Do(es.ctx, es.client)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("error indexing document: %s", res.String())
	}

	log.Printf("Document indexed successfully: %s", docID)
	return nil
}

// SearchPosts performs full-text search on posts
func (es *ElasticSearch) SearchPosts(query string) ([]map[string]interface{}, error) {
	// Build the search query
	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"title", "content"},
			},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(searchQuery); err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	// Perform search
	res, err := es.client.Search(
		es.client.Search.WithContext(es.ctx),
		es.client.Search.WithIndex("posts"),
		es.client.Search.WithBody(&buf),
		es.client.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search error: %s", res.String())
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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

	return posts, nil
}

// GetRelatedPosts finds posts with similar tags
func (es *ElasticSearch) GetRelatedPosts(currentPostID int, tags []string) []models.Related {
	if len(tags) == 0 {
		return []models.Related{}
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

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(searchQuery); err != nil {
		log.Printf("Failed to encode related posts query: %v", err)
		return []models.Related{}
	}

	res, err := es.client.Search(
		es.client.Search.WithContext(es.ctx),
		es.client.Search.WithIndex("posts"),
		es.client.Search.WithBody(&buf),
	)
	if err != nil {
		log.Printf("Error searching related posts: %v", err)
		return []models.Related{}
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Printf("Error searching related posts: %s", res.String())
		return []models.Related{}
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		log.Printf("Failed to parse related posts response: %v", err)
		return []models.Related{}
	}

	var relatedPosts []models.Related
	if hits, ok := result["hits"].(map[string]interface{}); ok {
		if hitsArray, ok := hits["hits"].([]interface{}); ok {
			for _, hit := range hitsArray {
				if hitMap, ok := hit.(map[string]interface{}); ok {
					source := hitMap["_source"].(map[string]interface{})

					related := models.Related{
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
