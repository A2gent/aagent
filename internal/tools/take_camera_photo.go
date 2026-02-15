package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCameraFormat         = "jpg"
	defaultCameraDirKey         = "AAGENT_CAMERA_OUTPUT_DIR"
	defaultCameraDir            = "/tmp"
	defaultCameraIndexKey       = "AAGENT_CAMERA_INDEX"
	defaultCameraIndex          = 1
	defaultInlineMaxBytes int64 = 2 * 1024 * 1024
)

type TakeCameraPhotoParams struct {
	OutputPath     string `json:"output_path,omitempty"`
	OutputDir      string `json:"output_dir,omitempty"`
	Filename       string `json:"filename,omitempty"`
	Format         string `json:"format,omitempty"` // png | jpg | jpeg
	CameraIndex    int    `json:"camera_index,omitempty"`
	ReturnInline   *bool  `json:"return_inline,omitempty"`
	InlineMaxBytes int64  `json:"inline_max_bytes,omitempty"`
}

type TakeCameraPhotoTool struct {
	workDir string
}

func NewTakeCameraPhotoTool(workDir string) *TakeCameraPhotoTool {
	return &TakeCameraPhotoTool{workDir: workDir}
}

func (t *TakeCameraPhotoTool) Name() string {
	return "take_camera_photo_tool"
}

func (t *TakeCameraPhotoTool) Description() string {
	return `Capture a photo from a camera device.
Supports selecting a specific camera index and configurable output path.
Can also return inline image metadata for in-memory multimodal model handoff.
On macOS this is captured natively by the Go binary (AVFoundation via cgo).`
}

func (t *TakeCameraPhotoTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"output_path": map[string]interface{}{
				"type":        "string",
				"description": "Optional output file path, or directory path if no extension is provided. Relative paths are resolved from project workdir.",
			},
			"output_dir": map[string]interface{}{
				"type":        "string",
				"description": "Optional output directory. Ignored when output_path points to a file.",
			},
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "Optional filename. If omitted, a timestamp-based name is used.",
			},
			"format": map[string]interface{}{
				"type":        "string",
				"description": "Image format: jpg or png (default: jpg).",
				"enum":        []string{"png", "jpg", "jpeg"},
			},
			"camera_index": map[string]interface{}{
				"type":        "integer",
				"description": "1-based camera index. If omitted, uses Tools UI default (or 1).",
			},
			"return_inline": map[string]interface{}{
				"type":        "boolean",
				"description": "When true, includes inline image metadata in the result for in-memory model handoff (default: true).",
			},
			"inline_max_bytes": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum bytes allowed for inline base64 payload (default: 2097152).",
			},
		},
	}
}

func (t *TakeCameraPhotoTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var p TakeCameraPhotoParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	format, err := normalizeCameraFormat(p.Format, p.Filename, p.OutputPath)
	if err != nil {
		return &Result{Success: false, Error: err.Error()}, nil
	}

	cameraIndex := p.CameraIndex
	if cameraIndex <= 0 {
		cameraIndex = configuredDefaultCameraIndex()
	}
	if cameraIndex <= 0 {
		cameraIndex = defaultCameraIndex
	}

	absPath, err := t.resolveOutputPath(p, format)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve output path: %w", err)
	}

	if err := captureCameraPhoto(ctx, cameraIndex, format, absPath); err != nil {
		return &Result{Success: false, Error: err.Error()}, nil
	}

	info, statErr := os.Stat(absPath)
	if statErr != nil {
		return nil, fmt.Errorf("camera capture completed but output file is missing: %w", statErr)
	}

	returnInline := true
	if p.ReturnInline != nil {
		returnInline = *p.ReturnInline
	}

	inlineMaxBytes := p.InlineMaxBytes
	if inlineMaxBytes <= 0 {
		inlineMaxBytes = defaultInlineMaxBytes
	}

	payload := map[string]interface{}{
		"path":         absPath,
		"camera_index": cameraIndex,
		"format":       format,
		"bytes":        info.Size(),
	}
	if rel, err := filepath.Rel(t.workDir, absPath); err == nil {
		payload["relative_path"] = rel
	}

	metadata := map[string]interface{}{
		"image_file": map[string]interface{}{
			"path":         absPath,
			"format":       format,
			"bytes":        info.Size(),
			"camera_index": cameraIndex,
			"source_tool":  t.Name(),
		},
	}

	if returnInline && info.Size() <= inlineMaxBytes {
		mediaType := "image/jpeg"
		if format == "png" {
			mediaType = "image/png"
		}
		metadata["image_inline"] = map[string]interface{}{
			"path":         absPath,
			"media_type":   mediaType,
			"max_bytes":    inlineMaxBytes,
			"source_tool":  t.Name(),
			"camera_index": cameraIndex,
		}
		payload["inline_available"] = true
	} else {
		payload["inline_available"] = false
		if returnInline && info.Size() > inlineMaxBytes {
			payload["inline_skipped_reason"] = fmt.Sprintf("image is %d bytes, exceeds inline_max_bytes=%d", info.Size(), inlineMaxBytes)
		}
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode result: %w", err)
	}

	return &Result{
		Success:  true,
		Output:   string(out),
		Metadata: metadata,
	}, nil
}

func normalizeCameraFormat(raw string, filename string, outputPath string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format == "" {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(filename)), "."))
		if ext == "" {
			ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(outputPath)), "."))
		}
		switch ext {
		case "png":
			format = "png"
		case "jpg", "jpeg":
			format = "jpg"
		default:
			format = defaultCameraFormat
		}
	}

	switch format {
	case "jpeg":
		format = "jpg"
	case "png", "jpg":
	default:
		return "", fmt.Errorf("unsupported format %q (expected png or jpg)", format)
	}
	return format, nil
}

func configuredDefaultCameraIndex() int {
	raw := strings.TrimSpace(os.Getenv(defaultCameraIndexKey))
	if raw == "" {
		return 0
	}
	idx, err := strconv.Atoi(raw)
	if err != nil || idx <= 0 {
		return 0
	}
	return idx
}

func (t *TakeCameraPhotoTool) resolveOutputPath(p TakeCameraPhotoParams, format string) (string, error) {
	resolvePath := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return ""
		}
		if filepath.IsAbs(raw) {
			return raw
		}
		return filepath.Join(t.workDir, raw)
	}

	filename := strings.TrimSpace(p.Filename)
	if filename == "" {
		filename = fmt.Sprintf("camera-%s.%s", time.Now().Format("20060102-150405"), format)
	} else if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), "."); ext == "" {
		filename += "." + format
	}

	outputPath := resolvePath(p.OutputPath)
	if outputPath != "" {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(outputPath)), ".")
		if ext != "" {
			if ext == "jpeg" {
				ext = "jpg"
			}
			if ext != format {
				return "", fmt.Errorf("output_path extension .%s does not match format %q", ext, format)
			}
			if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
				return "", err
			}
			return outputPath, nil
		}
		if err := os.MkdirAll(outputPath, 0755); err != nil {
			return "", err
		}
		return filepath.Join(outputPath, filename), nil
	}

	outputDir := resolvePath(p.OutputDir)
	if outputDir == "" {
		envDir := strings.TrimSpace(os.Getenv(defaultCameraDirKey))
		if envDir != "" {
			outputDir = resolvePath(envDir)
		} else {
			outputDir = defaultCameraDir
		}
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(outputDir, filename), nil
}

func captureCameraPhoto(ctx context.Context, cameraIndex int, format string, outputPath string) error {
	switch runtime.GOOS {
	case "darwin":
		return captureCameraPhotoDarwin(cameraIndex, format, outputPath)
	case "linux":
		return captureCameraPhotoLinux(ctx, cameraIndex, format, outputPath)
	case "windows":
		return captureCameraPhotoWindows(ctx, cameraIndex, format, outputPath)
	default:
		return fmt.Errorf("camera capture is not supported on %s", runtime.GOOS)
	}
}

func captureCameraPhotoLinux(ctx context.Context, cameraIndex int, format string, outputPath string) error {
	deviceIndex := cameraIndex - 1
	if deviceIndex < 0 {
		deviceIndex = 0
	}
	devicePath := fmt.Sprintf("/dev/video%d", deviceIndex)

	if _, err := exec.LookPath("ffmpeg"); err == nil {
		args := []string{
			"-y",
			"-loglevel", "error",
			"-f", "v4l2",
			"-i", devicePath,
		}
		if format == "png" {
			args = append(args, "-vcodec", "png")
		}
		args = append(args, "-frames:v", "1", outputPath)
		return runCommand(ctx, "ffmpeg", args...)
	}

	if _, err := exec.LookPath("fswebcam"); err == nil {
		args := []string{
			"-d", devicePath,
			"--no-banner",
			outputPath,
		}
		return runCommand(ctx, "fswebcam", args...)
	}

	return fmt.Errorf("no supported camera binary found on linux (tried ffmpeg, fswebcam)")
}

func captureCameraPhotoWindows(ctx context.Context, cameraIndex int, format string, outputPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("camera capture on windows requires ffmpeg in PATH")
	}

	cameras, err := listCamerasWindowsFFmpeg(ctx)
	if err != nil {
		return fmt.Errorf("failed to enumerate cameras via ffmpeg: %w", err)
	}
	if len(cameras) == 0 {
		return fmt.Errorf("no camera devices reported by ffmpeg")
	}
	if cameraIndex <= 0 || cameraIndex > len(cameras) {
		return fmt.Errorf("camera_index out of range: %d (available: %d)", cameraIndex, len(cameras))
	}

	deviceName := cameras[cameraIndex-1]
	args := []string{
		"-y",
		"-loglevel", "error",
		"-f", "dshow",
		"-i", "video=" + deviceName,
	}
	if format == "png" {
		args = append(args, "-vcodec", "png")
	}
	args = append(args, "-frames:v", "1", outputPath)
	return runCommand(ctx, "ffmpeg", args...)
}

func listCamerasWindowsFFmpeg(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	out, err := cmd.CombinedOutput()
	if len(out) == 0 && err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	cameras := make([]string, 0, len(lines))
	inVideoDevices := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "directshow video devices") {
			inVideoDevices = true
			continue
		}
		if strings.Contains(lower, "directshow audio devices") {
			inVideoDevices = false
			continue
		}
		if !inVideoDevices || !strings.Contains(trimmed, "[dshow") || !strings.Contains(trimmed, "\"") {
			continue
		}
		start := strings.Index(trimmed, "\"")
		end := strings.LastIndex(trimmed, "\"")
		if start >= 0 && end > start {
			name := strings.TrimSpace(trimmed[start+1 : end])
			if name != "" {
				cameras = append(cameras, name)
			}
		}
	}

	unique := make([]string, 0, len(cameras))
	seen := map[string]struct{}{}
	for _, name := range cameras {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		unique = append(unique, name)
	}
	return unique, nil
}

var _ Tool = (*TakeCameraPhotoTool)(nil)
