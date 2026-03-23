package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/ghostwriter/ghostwriter/internal/collector"
	"github.com/ghostwriter/ghostwriter/internal/config"
	"github.com/ghostwriter/ghostwriter/internal/embedder"
	"github.com/spf13/cobra"
)

func newIngestCmd() *cobra.Command {
	var (
		reset          bool
		noDashes       bool
		qdrantURL      string
		collectionName string
	)

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Embed and upsert writing samples into Qdrant",
		Long: `Process collected writing samples and store them in the Qdrant vector database.

Reads the JSONL corpus file, builds document text with metadata, and upserts
the embedded vectors into Qdrant for semantic search at generation time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// CLI flags override config values.
			if qdrantURL == "" {
				qdrantURL = cfg.Qdrant.URL
			}
			if collectionName == "" {
				collectionName = cfg.Qdrant.CollectionName
			}
			normalizeDashes := cfg.Style.NormalizeDashes
			if noDashes {
				normalizeDashes = false
			}

			// Load corpus file.
			corpusFile := filepath.Join(cfg.Corpus.Dir, "reviews.jsonl")
			records, err := loadCorpus(corpusFile)
			if err != nil {
				return err
			}

			log.Info("loaded corpus",
				"file", corpusFile,
				"records", len(records),
			)

			e := embedder.NewEmbedder(embedder.EmbedderOpts{
				QdrantURL:       qdrantURL,
				CollectionName:  collectionName,
				EmbeddingModel:  cfg.Qdrant.EmbeddingModel,
				NormalizeDashes: normalizeDashes,
			})

			result, err := e.Ingest(cmd.Context(), records, reset, func(batch, total int) {
				log.Info("upserting", "progress", fmt.Sprintf("%d/%d", batch, total))
			})
			if err != nil {
				return fmt.Errorf("ingestion failed: %w", err)
			}

			log.Info("ingestion complete",
				"total_samples", result.TotalSamples,
				"filtered", result.FilteredCount,
				"ingested", result.IngestedCount,
			)

			if result.CollectionInfo != nil {
				log.Info("collection info",
					"points", result.CollectionInfo.PointsCount,
					"vector_size", result.CollectionInfo.VectorSize,
					"distance", result.CollectionInfo.Distance,
				)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&reset, "reset", false, "Delete and recreate the collection before ingesting")
	cmd.Flags().BoolVar(&noDashes, "no-normalize-dashes", false, "Disable dash-to-comma normalization")
	cmd.Flags().StringVar(&qdrantURL, "qdrant-url", "", "Qdrant server URL (overrides config)")
	cmd.Flags().StringVar(&collectionName, "collection", "", "Qdrant collection name (overrides config)")

	return cmd
}

// loadCorpus reads a JSONL file and returns the parsed review records.
func loadCorpus(path string) ([]collector.ReviewRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("corpus file not found: %s\nRun 'gw collect github' first to collect writing samples", path)
		}
		return nil, fmt.Errorf("unable to open corpus file: %w", err)
	}
	defer f.Close()

	var records []collector.ReviewRecord
	dec := json.NewDecoder(f)
	for dec.More() {
		var rec collector.ReviewRecord
		if err := dec.Decode(&rec); err != nil {
			// Skip malformed lines, matching the Python behavior.
			log.Debug("skipping malformed record", "error", err)
			continue
		}
		records = append(records, rec)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no records found in corpus file: %s", path)
	}

	return records, nil
}
