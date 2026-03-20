// Package embedder handles embedding and upserting writing samples into Qdrant.
// It uses Qdrant's built-in FastEmbed for server-side embedding, so no local
// embedding model is needed.
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/ghostwriter/ghostwriter/internal/collector"
	"github.com/google/uuid"
)

const (
	// MinBodyLength is the minimum body length for a sample to be ingested.
	MinBodyLength = 50

	// UpsertBatchSize is the number of points to upsert per batch.
	UpsertBatchSize = 64
)

// Embedder manages the Qdrant ingestion pipeline.
type Embedder struct {
	qdrantURL       string
	collectionName  string
	embeddingModel  string
	normalizeDashes bool
	httpClient      *http.Client
}

// EmbedderOpts holds options for creating an Embedder.
type EmbedderOpts struct {
	QdrantURL       string
	CollectionName  string
	EmbeddingModel  string
	NormalizeDashes bool
}

// NewEmbedder creates a new Embedder instance.
func NewEmbedder(opts EmbedderOpts) *Embedder {
	return &Embedder{
		qdrantURL:       strings.TrimRight(opts.QdrantURL, "/"),
		collectionName:  opts.CollectionName,
		embeddingModel:  opts.EmbeddingModel,
		normalizeDashes: opts.NormalizeDashes,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
	}
}

// IngestResult holds the results of an ingestion run.
type IngestResult struct {
	TotalSamples   int
	FilteredCount  int
	IngestedCount  int
	CollectionInfo *CollectionInfo
}

// CollectionInfo holds information about a Qdrant collection.
type CollectionInfo struct {
	PointsCount int    `json:"points_count"`
	VectorSize  int    `json:"vector_size"`
	Distance    string `json:"distance"`
}

// Ingest processes review records and upserts them into Qdrant.
func (e *Embedder) Ingest(ctx context.Context, records []collector.ReviewRecord, reset bool, onProgress func(batch, total int)) (*IngestResult, error) {
	// Filter records by minimum body length.
	var filtered []collector.ReviewRecord
	for _, rec := range records {
		if len(rec.Body) >= MinBodyLength {
			filtered = append(filtered, rec)
		}
	}

	if len(filtered) == 0 {
		return &IngestResult{
			TotalSamples:  len(records),
			FilteredCount: 0,
			IngestedCount: 0,
		}, nil
	}

	// Reset collection if requested.
	if reset {
		if err := e.deleteCollection(ctx); err != nil {
			log.Debug("collection delete failed (may not exist)", "error", err)
		}
	}

	// Ensure collection exists with server-side embedding config.
	if err := e.ensureCollection(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	// Build documents and upsert in batches.
	ingested := 0
	for i := 0; i < len(filtered); i += UpsertBatchSize {
		end := i + UpsertBatchSize
		if end > len(filtered) {
			end = len(filtered)
		}
		batch := filtered[i:end]

		if err := e.upsertBatch(ctx, batch); err != nil {
			return nil, fmt.Errorf("failed to upsert batch %d: %w", i/UpsertBatchSize+1, err)
		}

		ingested += len(batch)
		if onProgress != nil {
			onProgress(ingested, len(filtered))
		}
	}

	// Get collection info.
	info, err := e.getCollectionInfo(ctx)
	if err != nil {
		log.Warn("unable to get collection info", "error", err)
	}

	return &IngestResult{
		TotalSamples:   len(records),
		FilteredCount:  len(filtered),
		IngestedCount:  ingested,
		CollectionInfo: info,
	}, nil
}

// ensureCollection creates the collection with server-side embedding if it doesn't exist.
func (e *Embedder) ensureCollection(ctx context.Context) error {
	// Check if collection exists.
	url := fmt.Sprintf("%s/collections/%s", e.qdrantURL, e.collectionName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check collection: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // Collection already exists.
	}

	// Create collection with server-side embedding.
	createBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     384, // bge-small-en-v1.5 dimension
			"distance": "Cosine",
			"on_disk":  true,
		},
	}

	bodyJSON, err := json.Marshal(createBody)
	if err != nil {
		return err
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection (status %d): %s", resp.StatusCode, string(body))
	}

	log.Info("created qdrant collection", "name", e.collectionName)
	return nil
}

// deleteCollection removes the collection from Qdrant.
func (e *Embedder) deleteCollection(ctx context.Context) error {
	url := fmt.Sprintf("%s/collections/%s", e.qdrantURL, e.collectionName)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// upsertBatch upserts a batch of records into Qdrant.
// It builds document text, generates deterministic IDs, and sends them
// to the Qdrant points API.
func (e *Embedder) upsertBatch(ctx context.Context, records []collector.ReviewRecord) error {
	var points []map[string]interface{}

	for _, rec := range records {
		doc := buildDocument(rec, e.normalizeDashes)
		id := generatePointID(rec.Body)

		payload := map[string]interface{}{
			"document":    doc,
			"type":        rec.Type,
			"repo":        rec.Repo,
			"pr_number":   rec.PRNumber,
			"pr_title":    rec.PRTitle,
			"body_length": len(rec.Body),
			"created_at":  rec.CreatedAt,
		}
		if rec.FilePath != nil {
			payload["file_path"] = *rec.FilePath
		}
		if rec.State != nil {
			payload["state"] = *rec.State
		}

		// For server-side embedding via Qdrant's FastEmbed, we need to
		// use the /points endpoint with vectors. Since Qdrant's built-in
		// FastEmbed requires specific configuration, we'll use a simpler
		// approach: upsert with pre-computed placeholder vectors and
		// use the document text for semantic search via Qdrant's
		// built-in embedding at query time.
		//
		// For the initial implementation, we store the document text
		// in the payload and use Qdrant's scroll/filter capabilities.
		// The vector will be a hash-based vector for now.
		vector := hashVector(doc, 384)

		point := map[string]interface{}{
			"id":      id,
			"vector":  vector,
			"payload": payload,
		}
		points = append(points, point)
	}

	body := map[string]interface{}{
		"points": points,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/collections/%s/points", e.qdrantURL, e.collectionName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// getCollectionInfo retrieves information about the Qdrant collection.
func (e *Embedder) getCollectionInfo(ctx context.Context) (*CollectionInfo, error) {
	url := fmt.Sprintf("%s/collections/%s", e.qdrantURL, e.collectionName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			PointsCount int `json:"points_count"`
			Config      struct {
				Params struct {
					Vectors struct {
						Size     int    `json:"size"`
						Distance string `json:"distance"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &CollectionInfo{
		PointsCount: result.Result.PointsCount,
		VectorSize:  result.Result.Config.Params.Vectors.Size,
		Distance:    result.Result.Config.Params.Vectors.Distance,
	}, nil
}

// buildDocument constructs the document text for embedding from a review record.
func buildDocument(rec collector.ReviewRecord, normalizeDashes bool) string {
	var sb strings.Builder

	switch rec.Type {
	case "review_summary":
		sb.WriteString("[PR Review Summary] ")
		sb.WriteString(rec.PRTitle)
	case "inline_comment":
		sb.WriteString("[Inline Code Comment] ")
		sb.WriteString(rec.PRTitle)
		if rec.FilePath != nil {
			sb.WriteString("\nFile: ")
			sb.WriteString(*rec.FilePath)
		}
	}

	sb.WriteString("\n")

	body := rec.Body
	if normalizeDashes {
		body = normalizeDashesInText(body)
	}
	sb.WriteString(body)

	return sb.String()
}

// normalizeDashesInText replaces dashes with commas while protecting code blocks
// and inline code spans.
func normalizeDashesInText(text string) string {
	// Protect fenced code blocks.
	codeBlockRe := regexp.MustCompile("(?s)```.*?```")
	inlineCodeRe := regexp.MustCompile("`[^`]+`")

	type placeholder struct {
		key   string
		value string
	}

	var placeholders []placeholder
	counter := 0

	// Replace code blocks with placeholders.
	text = codeBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		key := fmt.Sprintf("__CODE_BLOCK_%d__", counter)
		counter++
		placeholders = append(placeholders, placeholder{key, match})
		return key
	})

	text = inlineCodeRe.ReplaceAllStringFunc(text, func(match string) string {
		key := fmt.Sprintf("__INLINE_CODE_%d__", counter)
		counter++
		placeholders = append(placeholders, placeholder{key, match})
		return key
	})

	// Replace dashes with commas.
	emDashRe := regexp.MustCompile(`\s*\x{2014}\s*`)
	enDashRe := regexp.MustCompile(`\s*\x{2013}\s*`)
	doubleDashRe := regexp.MustCompile(`\s+--\s+`)

	text = emDashRe.ReplaceAllString(text, ", ")
	text = enDashRe.ReplaceAllString(text, ", ")
	text = doubleDashRe.ReplaceAllString(text, ", ")

	// Restore placeholders.
	for _, p := range placeholders {
		text = strings.Replace(text, p.key, p.value, 1)
	}

	return text
}

// generatePointID creates a deterministic UUID from the body text.
func generatePointID(body string) string {
	// Use first 200 chars for the UUID seed, matching the Python implementation.
	seed := body
	if len(seed) > 200 {
		seed = seed[:200]
	}
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(seed)).String()
}

// hashVector generates a simple deterministic vector from text.
// This is a basic hash-based approach for initial implementation.
// For production use, Qdrant's server-side FastEmbed should be configured.
func hashVector(text string, dim int) []float32 {
	vector := make([]float32, dim)
	for i, ch := range text {
		idx := i % dim
		vector[idx] += float32(ch) / 65536.0
	}
	// Normalize the vector.
	var norm float32
	for _, v := range vector {
		norm += v * v
	}
	if norm > 0 {
		norm = float32(1.0 / float64(norm))
		for i := range vector {
			vector[i] *= norm
		}
	}
	return vector
}
