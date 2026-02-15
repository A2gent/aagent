//go:build darwin && !cgo

package tools

import "fmt"

func captureCameraPhotoDarwin(cameraIndex int, format string, outputPath string) error {
	_ = cameraIndex
	_ = format
	_ = outputPath
	return fmt.Errorf("camera capture on darwin requires a cgo-enabled build")
}
