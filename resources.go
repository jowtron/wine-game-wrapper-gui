package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed resources
var embeddedResources embed.FS

// extractEmbeddedResources extracts the embedded resources to a temporary directory
// and returns the path. The caller is responsible for cleanup.
func extractEmbeddedResources() (string, error) {
	tmpDir, err := os.MkdirTemp("", "wgw-resources-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	err = fs.WalkDir(embeddedResources, "resources", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Strip the "resources/" prefix for the destination
		rel, _ := filepath.Rel("resources", path)
		destPath := filepath.Join(tmpDir, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		data, err := embeddedResources.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}

		return os.WriteFile(destPath, data, 0644)
	})

	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("extract resources: %w", err)
	}

	return tmpDir, nil
}
