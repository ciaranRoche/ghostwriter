// Package collector provides data collection from various sources.
// Currently supports GitHub PR review collection via GraphQL.
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// ReviewRecord represents a single writing sample collected from GitHub.
type ReviewRecord struct {
	Type      string  `json:"type"`
	Repo      string  `json:"repo"`
	PRNumber  int     `json:"pr_number"`
	PRTitle   string  `json:"pr_title"`
	Body      string  `json:"body"`
	State     *string `json:"state"`
	FilePath  *string `json:"file_path"`
	CreatedAt string  `json:"created_at"`
}

// GitHubCollector collects PR reviews from GitHub using GraphQL.
type GitHubCollector struct {
	client    *githubv4.Client
	username  string
	orgs      []string
	batchSize int
	maxPages  int
	outputDir string
}

// GitHubCollectorOpts holds options for creating a GitHubCollector.
type GitHubCollectorOpts struct {
	Token     string
	Username  string
	Orgs      []string
	BatchSize int
	MaxPages  int
	OutputDir string
}

// NewGitHubCollector creates a new GitHub review collector.
// If token is empty, it attempts to get a token from the gh CLI.
func NewGitHubCollector(opts GitHubCollectorOpts) (*GitHubCollector, error) {
	token := opts.Token
	if token == "" {
		var err error
		token, err = getGHToken()
		if err != nil {
			return nil, fmt.Errorf("no GitHub token available: set GITHUB_TOKEN or install gh CLI: %w", err)
		}
	}

	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), src)
	client := githubv4.NewClient(httpClient)

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}

	return &GitHubCollector{
		client:    client,
		username:  opts.Username,
		orgs:      opts.Orgs,
		batchSize: batchSize,
		maxPages:  maxPages,
		outputDir: opts.OutputDir,
	}, nil
}

// getGHToken attempts to retrieve a GitHub token from the gh CLI.
func getGHToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token failed: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh auth token returned empty string")
	}
	return token, nil
}

// prSearchQuery is the GraphQL query for searching PRs reviewed by a user.
// We use the GitHub search API with GraphQL to find PRs.
var prSearchQuery struct {
	Search struct {
		PageInfo struct {
			HasNextPage bool
			EndCursor   githubv4.String
		}
		Nodes []struct {
			PullRequest struct {
				Number     int
				Title      string
				CreatedAt  time.Time
				Repository struct {
					NameWithOwner string
				}
				Reviews struct {
					Nodes []struct {
						Body      string
						State     string
						CreatedAt time.Time
						Comments  struct {
							Nodes []struct {
								Body      string
								Path      string
								CreatedAt time.Time
							}
						} `graphql:"comments(first: 20)"`
					}
				} `graphql:"reviews(author: $username, first: 10)"`
			} `graphql:"... on PullRequest"`
		}
	} `graphql:"search(query: $query, type: ISSUE, first: $first, after: $after)"`
}

// CollectResult holds the results of a collection run.
type CollectResult struct {
	TotalRecords  int
	UniqueRecords int
	OutputFile    string
}

// Collect runs the full collection pipeline for all configured orgs.
func (c *GitHubCollector) Collect(ctx context.Context, onProgress func(org string, page int, found int)) (*CollectResult, error) {
	if err := os.MkdirAll(c.outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("unable to create output directory: %w", err)
	}

	outputFile := filepath.Join(c.outputDir, "reviews.jsonl")
	var allRecords []ReviewRecord

	for _, org := range c.orgs {
		org = strings.TrimSpace(org)
		if org == "" {
			continue
		}

		records, err := c.collectOrg(ctx, org, onProgress)
		if err != nil {
			log.Warn("error collecting from org, skipping", "org", org, "error", err)
			continue
		}
		allRecords = append(allRecords, records...)
	}

	// Deduplicate by body content.
	unique := deduplicateRecords(allRecords)

	// Write output file.
	f, err := os.Create(outputFile)
	if err != nil {
		return nil, fmt.Errorf("unable to create output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, rec := range unique {
		if err := enc.Encode(rec); err != nil {
			return nil, fmt.Errorf("error writing record: %w", err)
		}
	}

	return &CollectResult{
		TotalRecords:  len(allRecords),
		UniqueRecords: len(unique),
		OutputFile:    outputFile,
	}, nil
}

// collectOrg collects reviews from a single GitHub org.
func (c *GitHubCollector) collectOrg(ctx context.Context, org string, onProgress func(string, int, int)) ([]ReviewRecord, error) {
	var records []ReviewRecord
	query := fmt.Sprintf("type:pr reviewed-by:%s org:%s sort:updated-desc", c.username, org)

	var after *githubv4.String
	batchSize := githubv4.Int(int32(c.batchSize))

	for page := 1; page <= c.maxPages; page++ {
		variables := map[string]interface{}{
			"query":    githubv4.String(query),
			"first":    batchSize,
			"after":    after,
			"username": githubv4.String(c.username),
		}

		if err := c.client.Query(ctx, &prSearchQuery, variables); err != nil {
			return records, fmt.Errorf("graphql query failed for org %s page %d: %w", org, page, err)
		}

		for _, node := range prSearchQuery.Search.Nodes {
			pr := node.PullRequest
			repo := pr.Repository.NameWithOwner

			for _, review := range pr.Reviews.Nodes {
				// Extract review summary (skip empty and trivial comments).
				body := strings.TrimSpace(review.Body)
				if body != "" && !isTrivialComment(body) {
					state := review.State
					records = append(records, ReviewRecord{
						Type:      "review_summary",
						Repo:      repo,
						PRNumber:  pr.Number,
						PRTitle:   pr.Title,
						Body:      body,
						State:     &state,
						FilePath:  nil,
						CreatedAt: review.CreatedAt.Format(time.RFC3339),
					})
				}

				// Extract inline comments.
				for _, comment := range review.Comments.Nodes {
					commentBody := strings.TrimSpace(comment.Body)
					if commentBody != "" && len(commentBody) > 20 {
						path := comment.Path
						records = append(records, ReviewRecord{
							Type:      "inline_comment",
							Repo:      repo,
							PRNumber:  pr.Number,
							PRTitle:   pr.Title,
							Body:      commentBody,
							State:     nil,
							FilePath:  &path,
							CreatedAt: comment.CreatedAt.Format(time.RFC3339),
						})
					}
				}
			}
		}

		if onProgress != nil {
			onProgress(org, page, len(records))
		}

		if !prSearchQuery.Search.PageInfo.HasNextPage {
			break
		}
		cursor := prSearchQuery.Search.PageInfo.EndCursor
		after = &cursor

		// Rate limit courtesy delay.
		time.Sleep(500 * time.Millisecond)
	}

	return records, nil
}

// isTrivialComment returns true if a comment body is trivial (e.g., just "/lgtm").
func isTrivialComment(body string) bool {
	normalized := strings.ToLower(strings.TrimSpace(body))
	trivial := []string{"/lgtm", "lgtm", "lgtm!", "lg", "ship it"}
	for _, t := range trivial {
		if normalized == t {
			return true
		}
	}
	return false
}

// deduplicateRecords removes duplicate records based on body content,
// keeping the most recent version of each.
func deduplicateRecords(records []ReviewRecord) []ReviewRecord {
	seen := make(map[string]int) // body -> index in result
	var result []ReviewRecord

	for _, rec := range records {
		if idx, exists := seen[rec.Body]; exists {
			// Keep the more recent one.
			if rec.CreatedAt > result[idx].CreatedAt {
				result[idx] = rec
			}
		} else {
			seen[rec.Body] = len(result)
			result = append(result, rec)
		}
	}

	return result
}

// DetectUsername attempts to detect the GitHub username from the gh CLI.
func DetectUsername() (string, error) {
	cmd := exec.Command("gh", "api", "user", "--jq", ".login")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("unable to detect GitHub username: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// DetectOrgs attempts to detect the user's GitHub orgs from the gh CLI.
func DetectOrgs() ([]string, error) {
	cmd := exec.Command("gh", "api", "user/orgs", "--jq", ".[].login")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("unable to detect GitHub orgs: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var orgs []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			orgs = append(orgs, line)
		}
	}
	return orgs, nil
}
