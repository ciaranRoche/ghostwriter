// Package embedder handles embedding and upserting writing samples into Qdrant.
// It uses FastEmbed (via a thin Python subprocess) to generate embeddings that
// are compatible with mcp-server-qdrant's search, then upserts via Qdrant's
// REST API using the same named vector schema.
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

	// VectorDimension is the embedding dimension for all-MiniLM-L6-v2.
	VectorDimension = 384
)

// vectorName derives the Qdrant named vector key from the model name,
// matching mcp-server-qdrant's get_vector_name() convention:
// take the part after the last "/", lowercase it, prepend "fast-".
func vectorName(model string) string {
	parts := strings.Split(model, "/")
	name := parts[len(parts)-1]
	return "fast-" + strings.ToLower(name)
}

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
		httpClient:      &http.Client{Timeout: 60 * time.Second},
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

	// Build all document texts upfront.
	documents := make([]string, len(filtered))
	for i, rec := range filtered {
		documents[i] = buildDocument(rec, e.normalizeDashes)
	}

	// Generate embeddings via FastEmbed Python subprocess.
	log.Info("generating embeddings", "model", e.embeddingModel, "documents", len(documents))
	embeddings, err := e.embed(ctx, documents)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// Reset collection if requested.
	if reset {
		if err := e.deleteCollection(ctx); err != nil {
			log.Debug("collection delete failed (may not exist)", "error", err)
		}
	}

	// Ensure collection exists with the correct named vector config.
	if err := e.ensureCollection(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	// Upsert in batches.
	vecName := vectorName(e.embeddingModel)
	ingested := 0
	for i := 0; i < len(filtered); i += UpsertBatchSize {
		end := i + UpsertBatchSize
		if end > len(filtered) {
			end = len(filtered)
		}

		if err := e.upsertBatch(ctx, filtered[i:end], documents[i:end], embeddings[i:end], vecName); err != nil {
			return nil, fmt.Errorf("failed to upsert batch %d: %w", i/UpsertBatchSize+1, err)
		}

		ingested += end - i
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

// embed calls FastEmbed via a Python subprocess to generate embeddings.
// This uses passage_embed (not query_embed) to match how mcp-server-qdrant
// embeds documents for storage.
func (e *Embedder) embed(ctx context.Context, documents []string) ([][]float32, error) {
	// Write documents to a temp file as JSON array.
	tmpDir, err := os.MkdirTemp("", "gw-embed-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")

	inputData, err := json.Marshal(documents)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(inputPath, inputData, 0o644); err != nil {
		return nil, err
	}

	// Python script that uses FastEmbed's passage_embed, matching
	// mcp-server-qdrant's embedding approach.
	script := fmt.Sprintf(`
import json, sys
try:
    from fastembed import TextEmbedding
except ImportError:
    print("ERROR: fastembed not installed. Run: pip install fastembed", file=sys.stderr)
    sys.exit(1)

with open(%q) as f:
    docs = json.load(f)

model = TextEmbedding(%q)
embeddings = list(model.passage_embed(docs))
result = [e.tolist() for e in embeddings]

with open(%q, "w") as f:
    json.dump(result, f)
`, inputPath, e.embeddingModel, outputPath)

	cmd := exec.CommandContext(ctx, "python3", "-c", script)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("python3 embedding subprocess failed: %w\nMake sure fastembed is installed: pip install fastembed", err)
	}

	// Read embeddings output.
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedding output: %w", err)
	}

	var rawEmbeddings [][]float64
	if err := json.Unmarshal(outputData, &rawEmbeddings); err != nil {
		return nil, fmt.Errorf("failed to parse embeddings: %w", err)
	}

	// Convert float64 to float32.
	embeddings := make([][]float32, len(rawEmbeddings))
	for i, raw := range rawEmbeddings {
		embeddings[i] = make([]float32, len(raw))
		for j, v := range raw {
			embeddings[i][j] = float32(v)
		}
	}

	return embeddings, nil
}

// ensureCollection creates the collection with a named vector config matching
// mcp-server-qdrant's schema.
func (e *Embedder) ensureCollection(ctx context.Context) error {
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
		return nil
	}

	// Create collection with named vector, matching mcp-server-qdrant.
	vecName := vectorName(e.embeddingModel)
	createBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			vecName: map[string]interface{}{
				"size":     VectorDimension,
				"distance": "Cosine",
			},
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

	log.Info("created qdrant collection", "name", e.collectionName, "vector", vecName)
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

// upsertBatch upserts a batch of records into Qdrant using the named vector
// schema and payload format that mcp-server-qdrant expects.
func (e *Embedder) upsertBatch(ctx context.Context, records []collector.ReviewRecord, documents []string, embeddings [][]float32, vecName string) error {
	var points []map[string]interface{}

	for i, rec := range records {
		id := generatePointID(rec.Body)

		// mcp-server-qdrant expects payload with "document" and "metadata" fields.
		metadata := map[string]interface{}{
			"type":        rec.Type,
			"repo":        rec.Repo,
			"pr_number":   rec.PRNumber,
			"pr_title":    rec.PRTitle,
			"body_length": len(rec.Body),
			"created_at":  rec.CreatedAt,
		}
		if rec.FilePath != nil {
			metadata["file_path"] = *rec.FilePath
		}
		if rec.State != nil {
			metadata["state"] = *rec.State
		}

		point := map[string]interface{}{
			"id": id,
			"vector": map[string]interface{}{
				vecName: embeddings[i],
			},
			"payload": map[string]interface{}{
				"document": documents[i],
				"metadata": metadata,
			},
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get collection info (status %d)", resp.StatusCode)
	}

	var result struct {
		Result struct {
			PointsCount int `json:"points_count"`
			Config      struct {
				Params struct {
					Vectors map[string]struct {
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

	info := &CollectionInfo{
		PointsCount: result.Result.PointsCount,
	}

	// Extract vector info from the named vector config.
	vecName := vectorName(e.embeddingModel)
	if v, ok := result.Result.Config.Params.Vectors[vecName]; ok {
		info.VectorSize = v.Size
		info.Distance = v.Distance
	}

	return info, nil
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
	codeBlockRe := regexp.MustCompile("(?s)```.*?```")
	inlineCodeRe := regexp.MustCompile("`[^`]+`")

	type placeholder struct {
		key   string
		value string
	}

	var placeholders []placeholder
	counter := 0

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

	emDashRe := regexp.MustCompile(`\s*\x{2014}\s*`)
	enDashRe := regexp.MustCompile(`\s*\x{2013}\s*`)
	doubleDashRe := regexp.MustCompile(`\s+--\s+`)

	text = emDashRe.ReplaceAllString(text, ", ")
	text = enDashRe.ReplaceAllString(text, ", ")
	text = doubleDashRe.ReplaceAllString(text, ", ")

	for _, p := range placeholders {
		text = strings.Replace(text, p.key, p.value, 1)
	}

	return text
}

// generatePointID creates a deterministic UUID from the body text.
func generatePointID(body string) string {
	seed := body
	if len(seed) > 200 {
		seed = seed[:200]
	}
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(seed)).String()
}
