package utilities

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type MetadataEntry struct {
	ID  string `json:"n"`
	URL string `json:"u"`
}

// ReadMusicMetadata reads metadata.json and returns a map of IDs to URLs
func ReadMusicMetadata(dir string) (map[string]string, error) {
	metadataPath := filepath.Join(dir, "metadata.json")
	metadataFile, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata.json: %w", err)
	}

	var metadata []MetadataEntry
	if err := json.Unmarshal(metadataFile, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata.json: %w", err)
	}

	// Create a map for quick lookup
	metadataMap := make(map[string]string)
	for _, entry := range metadata {
		metadataMap[entry.ID] = entry.URL
	}

	return metadataMap, nil
}
