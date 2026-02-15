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
	Pattern           string   `json:"pattern"`
	Path              string   `json:"path,omitempty"`
	Include           string   `json:"include,omitempty"` // File pattern filter
	Exclude           []string `json:"exclude,omitempty"` // Relative path filters
	MaxResults        int      `json:"max_results,omitempty"`
	MaxMatchesPerFile int      `json:"max_matches_per_file,omitempty"`
	Mode              string   `json:"mode,omitempty"` // lines|files|count
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
Use mode=files or mode=count for compact outputs.
Use include/exclude and limits to reduce context usage.`
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
			"exclude": map[string]interface{}{
				"type":        "array",
				"description": "Exclude glob patterns matched against relative paths",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum output rows (default: 500)",
			},
			"max_matches_per_file": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum matches to emit per file (default: unlimited)",
			},
			"mode": map[string]interface{}{
				"type":        "string",
				"description": "Output mode: lines (default), files, count",
				"enum":        []string{"lines", "files", "count"},
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
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	if mode == "" {
		mode = "lines"
	}
	if mode != "lines" && mode != "files" && mode != "count" {
		return &Result{Success: false, Error: "mode must be one of: lines, files, count"}, nil
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
	fileCounts := make(map[string]int)
	maxResults := p.MaxResults
	if maxResults <= 0 {
		maxResults = maxGrepResults
	}
	if maxResults > maxGrepResults {
		maxResults = maxGrepResults
	}
	maxPerFile := p.MaxMatchesPerFile

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
		if isExcluded(relPath, p.Exclude) {
			continue
		}

		fileMatches, totalCount := t.searchFile(fullPath, relPath, re, info.ModTime().UnixNano(), maxPerFile, mode == "files")
		if totalCount > 0 {
			fileCounts[relPath] = totalCount
		}
		matches = append(matches, fileMatches...)

		if len(matches) >= maxResults {
			break
		}
	}

	if len(matches) == 0 && len(fileCounts) == 0 {
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
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	// Format output
	var lines []string
	switch mode {
	case "files":
		seen := make(map[string]struct{})
		for _, m := range matches {
			if _, ok := seen[m.file]; ok {
				continue
			}
			seen[m.file] = struct{}{}
			lines = append(lines, m.file)
		}
	case "count":
		paths := make([]string, 0, len(fileCounts))
		for path := range fileCounts {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			lines = append(lines, fmt.Sprintf("%s: %d", path, fileCounts[path]))
		}
	default:
		for _, m := range matches {
			content := m.content
			if len(content) > maxGrepLineLength {
				content = content[:maxGrepLineLength] + "..."
			}
			lines = append(lines, fmt.Sprintf("%s:%d: %s", m.file, m.line, content))
		}
	}

	output := strings.Join(lines, "\n")

	return &Result{
		Success: true,
		Output:  output,
	}, nil
}

func (t *GrepTool) searchFile(fullPath, relPath string, re *regexp.Regexp, modTime int64, maxMatches int, stopAtFirst bool) ([]grepMatch, int) {
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, 0
	}
	defer file.Close()

	var matches []grepMatch
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	totalCount := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if re.MatchString(line) {
			totalCount++
			if maxMatches <= 0 || len(matches) < maxMatches {
				matches = append(matches, grepMatch{
					file:    relPath,
					line:    lineNum,
					content: strings.TrimSpace(line),
					modTime: modTime,
				})
			}
			if stopAtFirst {
				break
			}
		}
	}

	return matches, totalCount
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
