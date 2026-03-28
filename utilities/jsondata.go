package utilities

import (
	"fmt"
	"path/filepath"
	"strings"
)

func RemoveLastCharacter(s string) string {
	r := []rune(s)
	if len(r) <= 2 {
		return s
	}

	return string(r[:len(r)-1])
}

// Be sure to call WriteFolderJSON before WriteFileJSON.
func WriteFolderJSON(folders []Folder, details bool) string {
	var data strings.Builder
	for _, f := range folders {
		if details {
			data.WriteString(fmt.Sprintf(`{"n":"%s","t":"d", "m":"%s", "s":"%s"},`, f.Name, f.Modified, f.Size))
		} else {
			data.WriteString(fmt.Sprintf(`{"n":"%s","t":"d"},`, f.Name))
		}
	}
	return data.String()
}

func WriteSearchEntryJSON(entries []SearchEntry) string {
	var data strings.Builder
	for _, e := range entries {
		data.WriteString(fmt.Sprintf(`{"n":"%s","t":"%s","p":"%s"},`, e.Name, e.Type, e.Path))
	}
	return data.String()
}

func WriteFileJSON(files []File, details bool, musicMetadata map[string]string) string {
	var data strings.Builder
	for _, f := range files {
		if f.Name == "index.html" {
			continue
		}

		var entry string
		if details {
			entry = fmt.Sprintf(`{"n":"%s","t":"f", "m":"%s", "s":"%s"`, f.Name, f.Modified, f.Size)
		} else {
			entry = fmt.Sprintf(`{"n":"%s","t":"f"`, f.Name)
		}

		// Add music URL if available.
		if musicMetadata != nil {
			id := strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
			if url, exists := musicMetadata[id]; exists && url != "" {
				entry += fmt.Sprintf(`,"u":"%s"`, url)
			}
		}

		entry += "},"
		data.WriteString(entry)
	}
	return data.String()
}
