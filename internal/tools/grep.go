package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	maxGrepResults    = 500
	maxGrepLineLength = 500
)

// GrepTool searches file contents using regex
type GrepTool struct {
	workDir string
}

// GrepParams defines parameters for the grep tool
type GrepParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Include string `json:"include,omitempty"` // File pattern filter
}

// NewGrepTool creates a new grep tool
func NewGrepTool(workDir string) *GrepTool {
	return &GrepTool{workDir: workDir}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Description() string {
	return `Search file contents using regular expressions.
Returns file paths and line numbers with at least one match.
Use the include parameter to filter files by pattern (e.g., "*.go", "*.{ts,tsx}").`
}

func (t *GrepTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Regular expression pattern to search for",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory to search in (optional, defaults to working directory)",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "File pattern to include (e.g., '*.go', '*.{ts,tsx}')",
			},
		},
		"required": []string{"pattern"},
	}
}

type grepMatch struct {
	file    string
	line    int
	content string
	modTime int64
}

func (t *GrepTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var p GrepParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if p.Pattern == "" {
		return &Result{Success: false, Error: "pattern is required"}, nil
	}

	// Compile regex
	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return &Result{Success: false, Error: fmt.Sprintf("invalid regex: %v", err)}, nil
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

	// Determine file pattern
	filePattern := "**/*"
	if p.Include != "" {
		filePattern = "**/" + p.Include
	}

	// Find files to search (follows symlinks by default)
	pattern := filepath.Join(basePath, filePattern)
	files, err := doublestar.FilepathGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}

	// Search files
	var matches []grepMatch

	for _, file := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// FilepathGlob returns absolute paths
		fullPath := file
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}

		// Skip binary files (simple heuristic)
		if isBinaryFile(fullPath) {
			continue
		}

		// Get relative path for display
		relPath, err := filepath.Rel(basePath, fullPath)
		if err != nil {
			relPath = file
		}

		fileMatches := t.searchFile(fullPath, relPath, re, info.ModTime().UnixNano())
		matches = append(matches, fileMatches...)

		if len(matches) >= maxGrepResults {
			break
		}
	}

	if len(matches) == 0 {
		return &Result{
			Success: true,
			Output:  "No matches found",
		}, nil
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	// Limit results
	if len(matches) > maxGrepResults {
		matches = matches[:maxGrepResults]
	}

	// Format output
	var lines []string
	for _, m := range matches {
		content := m.content
		if len(content) > maxGrepLineLength {
			content = content[:maxGrepLineLength] + "..."
		}
		lines = append(lines, fmt.Sprintf("%s:%d: %s", m.file, m.line, content))
	}

	output := strings.Join(lines, "\n")

	return &Result{
		Success: true,
		Output:  output,
	}, nil
}

func (t *GrepTool) searchFile(fullPath, relPath string, re *regexp.Regexp, modTime int64) []grepMatch {
	file, err := os.Open(fullPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var matches []grepMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if re.MatchString(line) {
			matches = append(matches, grepMatch{
				file:    relPath,
				line:    lineNum,
				content: strings.TrimSpace(line),
				modTime: modTime,
			})
		}
	}

	return matches
}

// isBinaryFile checks if a file appears to be binary
func isBinaryFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()

	// Read first 512 bytes
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return true
	}

	// Check for null bytes (common in binary files)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}

	return false
}

// Ensure GrepTool implements Tool
var _ Tool = (*GrepTool)(nil)
