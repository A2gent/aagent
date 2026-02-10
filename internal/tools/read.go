package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultReadLimit = 2000
	maxLineLength    = 2000
)

// ReadTool reads file contents
type ReadTool struct {
	workDir string
}

// ReadParams defines parameters for the read tool
type ReadParams struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"` // 0-based line offset
	Limit  int    `json:"limit,omitempty"`  // Number of lines to read
}

// NewReadTool creates a new read tool
func NewReadTool(workDir string) *ReadTool {
	return &ReadTool{workDir: workDir}
}

func (t *ReadTool) Name() string {
	return "read"
}

func (t *ReadTool) Description() string {
	return `Read file contents from the filesystem.
By default reads up to 2000 lines from the beginning.
Use offset and limit for reading specific sections of large files.
Line numbers in output start at 1.`
}

func (t *ReadTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute or relative path to the file",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "Line number to start reading from (0-based, optional)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of lines to read (default: 2000)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var p ReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if p.Path == "" {
		return &Result{Success: false, Error: "path is required"}, nil
	}

	// Resolve path
	path := p.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workDir, path)
	}

	// Check if file exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return &Result{Success: false, Error: fmt.Sprintf("file not found: %s", p.Path)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return &Result{Success: false, Error: fmt.Sprintf("%s is a directory", p.Path)}, nil
	}

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Set defaults
	offset := p.Offset
	limit := p.Limit
	if limit <= 0 {
		limit = defaultReadLimit
	}

	// Read lines
	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		lineNum++

		// Skip lines before offset
		if lineNum <= offset {
			continue
		}

		// Check limit
		if linesRead >= limit {
			break
		}

		line := scanner.Text()

		// Truncate long lines
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}

		// Format with line number (cat -n style)
		lines = append(lines, fmt.Sprintf("%6d\t%s", lineNum, line))
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	if len(lines) == 0 {
		return &Result{
			Success: true,
			Output:  "(empty file or no lines in range)",
		}, nil
	}

	output := strings.Join(lines, "\n")
	if linesRead == limit {
		output += fmt.Sprintf("\n\n(showing lines %d-%d, file may have more content)", offset+1, lineNum)
	}

	return &Result{
		Success: true,
		Output:  output,
	}, nil
}

// Ensure ReadTool implements Tool
var _ Tool = (*ReadTool)(nil)
