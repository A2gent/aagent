package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	defaultFindFilesLimit = 200
	maxFindFilesLimit     = 2000
)

// FindFilesTool finds files with include/exclude filters.
type FindFilesTool struct {
	workDir string
}

// FindFilesParams defines parameters for the find_files tool.
type FindFilesParams struct {
	Path       string   `json:"path,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
	MaxResults int      `json:"max_results,omitempty"`
	Sort       string   `json:"sort,omitempty"` // none|path|mtime
}

// NewFindFilesTool creates a new find_files tool.
func NewFindFilesTool(workDir string) *FindFilesTool {
	return &FindFilesTool{workDir: workDir}
}

func (t *FindFilesTool) Name() string {
	return "find_files"
}

func (t *FindFilesTool) Description() string {
	return `Find files with glob patterns and exclude filters.
Optimized for precise file discovery with compact output.
Use this before grep/read/edit to minimize context usage.`
}

func (t *FindFilesTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Base directory to search in (optional, defaults to working directory)",
			},
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Include glob pattern (default: '**/*')",
			},
			"exclude": map[string]interface{}{
				"type":        "array",
				"description": "Exclude glob patterns matched against relative paths",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results (default: 200, max: 2000)",
			},
			"sort": map[string]interface{}{
				"type":        "string",
				"description": "Sort mode: none, path, or mtime (default: path)",
				"enum":        []string{"none", "path", "mtime"},
			},
		},
	}
}

func (t *FindFilesTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var p FindFilesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	basePath := t.workDir
	if p.Path != "" {
		if filepath.IsAbs(p.Path) {
			basePath = p.Path
		} else {
			basePath = filepath.Join(t.workDir, p.Path)
		}
	}

	pattern := p.Pattern
	if strings.TrimSpace(pattern) == "" {
		pattern = "**/*"
	}

	limit := p.MaxResults
	if limit <= 0 {
		limit = defaultFindFilesLimit
	}
	if limit > maxFindFilesLimit {
		limit = maxFindFilesLimit
	}

	sortMode := strings.ToLower(strings.TrimSpace(p.Sort))
	if sortMode == "" {
		sortMode = "path"
	}
	if sortMode != "none" && sortMode != "path" && sortMode != "mtime" {
		return &Result{Success: false, Error: "sort must be one of: none, path, mtime"}, nil
	}

	globPattern := filepath.Join(basePath, pattern)
	matches, err := doublestar.FilepathGlob(globPattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}

	type fileResult struct {
		path    string
		modTime int64
	}
	results := make([]fileResult, 0, min(limit, len(matches)))
	totalIncluded := 0

	for _, match := range matches {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		info, err := os.Stat(match)
		if err != nil || info.IsDir() {
			continue
		}

		rel, err := filepath.Rel(basePath, match)
		if err != nil {
			rel = match
		}

		if isExcluded(rel, p.Exclude) {
			continue
		}

		totalIncluded++
		results = append(results, fileResult{path: rel, modTime: info.ModTime().UnixNano()})
	}

	switch sortMode {
	case "path":
		sort.Slice(results, func(i, j int) bool {
			return results[i].path < results[j].path
		})
	case "mtime":
		sort.Slice(results, func(i, j int) bool {
			return results[i].modTime > results[j].modTime
		})
	}

	if len(results) > limit {
		results = results[:limit]
	}

	if len(results) == 0 {
		return &Result{Success: true, Output: "No files found"}, nil
	}

	lines := make([]string, 0, len(results))
	for _, r := range results {
		lines = append(lines, r.path)
	}

	output := strings.Join(lines, "\n")
	if totalIncluded > len(results) {
		output += fmt.Sprintf("\n\n(showing %d of %d files)", len(results), totalIncluded)
	}

	return &Result{Success: true, Output: output}, nil
}

func isExcluded(path string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		ok, err := doublestar.PathMatch(pattern, path)
		if err == nil && ok {
			return true
		}
		if err == nil {
			ok, _ = doublestar.PathMatch("**/"+pattern, path)
			if ok {
				return true
			}
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ensure FindFilesTool implements Tool.
var _ Tool = (*FindFilesTool)(nil)
