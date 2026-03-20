// Package config manages ghostwriter configuration using Viper.
// Configuration is loaded from ~/.config/ghostwriter/config.yaml,
// environment variables, and CLI flags.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	// DefaultQdrantURL is the default Qdrant server address.
	DefaultQdrantURL = "http://127.0.0.1:6333"

	// DefaultCollectionName is the default Qdrant collection name.
	DefaultCollectionName = "writing-samples"

	// DefaultEmbeddingModel is the default FastEmbed model for server-side embedding.
	DefaultEmbeddingModel = "BAAI/bge-small-en-v1.5"

	// DefaultCorpusDir is the default directory for writing sample corpus files.
	DefaultCorpusDir = "rag/corpus"

	// DefaultBatchSize is the default number of PRs fetched per GraphQL page.
	DefaultBatchSize = 20

	// DefaultMaxPages is the default maximum pages to paginate per org.
	DefaultMaxPages = 10
)

// Config holds the ghostwriter configuration.
type Config struct {
	GitHub GitHubConfig `mapstructure:"github"`
	Qdrant QdrantConfig `mapstructure:"qdrant"`
	Corpus CorpusConfig `mapstructure:"corpus"`
	Style  StyleConfig  `mapstructure:"style"`
}

// GitHubConfig holds GitHub-related settings.
type GitHubConfig struct {
	Username  string   `mapstructure:"username"`
	Orgs      []string `mapstructure:"orgs"`
	Token     string   `mapstructure:"token"`
	BatchSize int      `mapstructure:"batch_size"`
	MaxPages  int      `mapstructure:"max_pages"`
}

// QdrantConfig holds Qdrant-related settings.
type QdrantConfig struct {
	URL            string `mapstructure:"url"`
	CollectionName string `mapstructure:"collection_name"`
	EmbeddingModel string `mapstructure:"embedding_model"`
}

// CorpusConfig holds corpus storage settings.
type CorpusConfig struct {
	Dir string `mapstructure:"dir"`
}

// StyleConfig holds style-related settings.
type StyleConfig struct {
	NormalizeDashes bool `mapstructure:"normalize_dashes"`
}

// ConfigDir returns the ghostwriter config directory path.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "ghostwriter"), nil
}

// ConfigPath returns the full path to the config file.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads and returns the ghostwriter configuration.
// It searches for config in ~/.config/ghostwriter/config.yaml,
// falls back to environment variables, and applies defaults.
func Load() (*Config, error) {
	setDefaults()
	bindEnvVars()

	configDir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configDir)

	// Also check current directory for a .env-style config
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		// Config file not found is fine, we'll use defaults and env vars
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	return &cfg, nil
}

// Save persists the current configuration to disk.
func Save(cfg *Config) error {
	configDir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("unable to create config directory: %w", err)
	}

	viper.Set("github.username", cfg.GitHub.Username)
	viper.Set("github.orgs", cfg.GitHub.Orgs)
	viper.Set("github.token", cfg.GitHub.Token)
	viper.Set("github.batch_size", cfg.GitHub.BatchSize)
	viper.Set("github.max_pages", cfg.GitHub.MaxPages)
	viper.Set("qdrant.url", cfg.Qdrant.URL)
	viper.Set("qdrant.collection_name", cfg.Qdrant.CollectionName)
	viper.Set("qdrant.embedding_model", cfg.Qdrant.EmbeddingModel)
	viper.Set("corpus.dir", cfg.Corpus.Dir)
	viper.Set("style.normalize_dashes", cfg.Style.NormalizeDashes)

	configPath := filepath.Join(configDir, "config.yaml")
	return viper.WriteConfigAs(configPath)
}

// setDefaults sets default values for all config keys.
func setDefaults() {
	viper.SetDefault("github.username", "")
	viper.SetDefault("github.orgs", []string{})
	viper.SetDefault("github.token", "")
	viper.SetDefault("github.batch_size", DefaultBatchSize)
	viper.SetDefault("github.max_pages", DefaultMaxPages)
	viper.SetDefault("qdrant.url", DefaultQdrantURL)
	viper.SetDefault("qdrant.collection_name", DefaultCollectionName)
	viper.SetDefault("qdrant.embedding_model", DefaultEmbeddingModel)
	viper.SetDefault("corpus.dir", DefaultCorpusDir)
	viper.SetDefault("style.normalize_dashes", true)
}

// bindEnvVars maps environment variables to config keys.
func bindEnvVars() {
	viper.SetEnvPrefix("GW")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Also support legacy env var names from the old .env format
	_ = viper.BindEnv("github.username", "GITHUB_USERNAME", "GW_GITHUB_USERNAME")
	_ = viper.BindEnv("github.orgs", "GITHUB_ORGS", "GW_GITHUB_ORGS")
	_ = viper.BindEnv("github.token", "GITHUB_TOKEN", "GW_GITHUB_TOKEN")
	_ = viper.BindEnv("qdrant.url", "QDRANT_URL", "GW_QDRANT_URL")
	_ = viper.BindEnv("qdrant.collection_name", "COLLECTION_NAME", "GW_QDRANT_COLLECTION_NAME")
}
