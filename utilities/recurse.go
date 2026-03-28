package utilities

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func IndexFolder(directory, boilerplate string, depth int, ignored []string, json, details, music, globalSearch bool, entries *[]SearchEntry) {
	fmt.Println("Indexing directory:", directory)

	bcopy := boilerplate // Used for recursive call.
	base := strings.Repeat("../", depth)
	boilerplate = strings.ReplaceAll(boilerplate, "[base]", base)

	// Inject global search link or clear the placeholder.
	if globalSearch {
		boilerplate = strings.ReplaceAll(boilerplate, "[global-search]", fmt.Sprintf(`<a href="%ssearch.html" class="gs-link">&#128269; Global Search</a>`, base))
	} else {
		boilerplate = strings.ReplaceAll(boilerplate, "[global-search]", "")
	}

	// Get list of folders and filter ignored ones.
	folders, _ := GetFolders(directory)
	var filteredFolders []Folder
	for _, folder := range folders {
		if !IsIgnored(folder.Name, ignored) {
			filteredFolders = append(filteredFolders, folder)
			IndexFolder(filepath.Join(directory, folder.Name), bcopy, depth+1, ignored, json, details, music, globalSearch, entries) // Recursive call.
		}
	}

	// Get list of files and filter ignored ones.
	fileList, _ := GetFiles(directory)
	var filteredFiles []File
	for _, file := range fileList {
		if !IsIgnored(file.Name, ignored) {
			filteredFiles = append(filteredFiles, file)
		}
	}

	// Accumulate entries for the global search index.
	if globalSearch && entries != nil {
		entryPath := strings.ReplaceAll(directory, "\\", "/")
		if entryPath == "." {
			entryPath = ""
		}
		for _, f := range filteredFolders {
			*entries = append(*entries, SearchEntry{Name: f.Name, Type: "d", Path: entryPath})
		}
		for _, f := range filteredFiles {
			*entries = append(*entries, SearchEntry{Name: f.Name, Type: "f", Path: entryPath})
		}
	}

	musicMetadata := make(map[string]string)
	if music {
		var err error
		musicMetadata, err = ReadMusicMetadata(directory)
		if err != nil {
			fmt.Println("Error reading music metadata:", err)
		}
	}

	// Modify HTML with directory data.
	boilerplate = WriteFolderName(directory, boilerplate)
	jsData := RemoveLastCharacter(WriteFolderJSON(filteredFolders, details) + WriteFileJSON(filteredFiles, details, musicMetadata))
	boilerplate = strings.ReplaceAll(boilerplate, "[data]", fmt.Sprintf("const d = [%s];", jsData))

	// Optionally write data.json file.
	if json {
		jsonPath := filepath.Join(directory, "data.json")
		err := os.WriteFile(jsonPath, []byte(fmt.Sprintf("[%s]", jsData)), 0644)
		if err != nil {
			fmt.Println("Error writing file:", err)
		}
	}

	// Write index.html.
	indexPath := filepath.Join(directory, "index.html")
	err := os.WriteFile(indexPath, []byte(boilerplate), 0644)
	if err != nil {
		fmt.Println("Error writing file:", err)
	}
}

// WriteSearchPage fills the [data] placeholder in htmlTemplate with the aggregated
// search index and writes search.html to the root of the traced directory.
func WriteSearchPage(htmlTemplate string, entries []SearchEntry) {
	jsData := RemoveLastCharacter(WriteSearchEntryJSON(entries))
	content := strings.ReplaceAll(htmlTemplate, "[data]", fmt.Sprintf("const sd = [%s];", jsData))
	err := os.WriteFile("./search.html", []byte(content), 0644)
	if err != nil {
		fmt.Println("Error writing search.html:", err)
	}
}
