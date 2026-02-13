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

const maxGlobResults = 1000

// GlobTool finds files by pattern
type GlobTool struct {
	workDir string
}

// GlobParams defines parameters for the glob tool
type GlobParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// NewGlobTool creates a new glob tool
func NewGlobTool(workDir string) *GlobTool {
	return &GlobTool{workDir: workDir}
}

func (t *GlobTool) Name() string {
	return "glob"
}

func (t *GlobTool) Description() string {
	return `Find files by pattern matching using glob patterns.
Supports patterns like "**/*.go", "src/**/*.ts", "*.json".
Returns matching file paths sorted by modification time (newest first).`
}

func (t *GlobTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern to match files (e.g., '**/*.go', 'src/**/*.ts')",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Base directory to search in (optional, defaults to working directory)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var p GlobParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if p.Pattern == "" {
		return &Result{Success: false, Error: "pattern is required"}, nil
	}

	// Determine base path
	basePath := t.workDir
	if p.Path != "" {
		if filepath.IsAbs(p.Path) {
			basePath = p.Path
		} else {
			basePath = filepath.Join(t.workDir, p.Path)
		}
	}

	// Use filesystem for globbing (follows symlinks by default)
	pattern := filepath.Join(basePath, p.Pattern)
	matches, err := doublestar.FilepathGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}

	// Convert absolute paths to relative paths from basePath
	for i, match := range matches {
		rel, err := filepath.Rel(basePath, match)
		if err != nil {
			rel = match
		}
		matches[i] = rel
	}

	if len(matches) == 0 {
		return &Result{
			Success: true,
			Output:  "No files found matching pattern",
		}, nil
	}

	// Get file info for sorting
	type fileInfo struct {
		path    string
		modTime int64
	}
	files := make([]fileInfo, 0, len(matches))

	for _, match := range matches {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		fullPath := filepath.Join(basePath, match)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		// Skip directories
		if info.IsDir() {
			continue
		}

		files = append(files, fileInfo{
			path:    match,
			modTime: info.ModTime().UnixNano(),
		})
	}

	// Sort by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime > files[j].modTime
	})

	// Limit results
	if len(files) > maxGlobResults {
		files = files[:maxGlobResults]
	}

	// Build output
	var paths []string
	for _, f := range files {
		paths = append(paths, f.path)
	}

	output := strings.Join(paths, "\n")
	if len(matches) > maxGlobResults {
		output += fmt.Sprintf("\n\n(showing %d of %d matches)", maxGlobResults, len(matches))
	}

	return &Result{
		Success: true,
		Output:  output,
	}, nil
}

// Ensure GlobTool implements Tool
var _ Tool = (*GlobTool)(nil)
