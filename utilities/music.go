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

func ResolveExternalLinks(dir string, music bool, catalog *Catalog) (map[string]string, error) {
	links := make(map[string]string)
	var firstErr error

	if music {
		musicMetadata, err := ReadMusicMetadata(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				firstErr = err
			}
		} else {
			for key, value := range musicMetadata {
				if value != "" {
					links[key] = value
				}
			}
		}
	}

	if catalog != nil {
		catalogLinks, err := catalog.GetExternalLinks(dir)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			for key, value := range catalogLinks {
				if value != "" {
					links[key] = value
				}
			}
		}
	}

	if len(links) == 0 {
		return nil, firstErr
	}

	return links, firstErr
}
