package utilities

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func IndexFolder(directory, boilerplate string, depth int, ignored []string, json, details, music, globalSearch bool, catalog *Catalog, writeFiles bool) {
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
		folderPath := filepath.Join(directory, folder.Name)
		if !IsIgnored(folder.Name, ignored) && !shouldSkipCatalogPath(catalog, folderPath) {
			filteredFolders = append(filteredFolders, folder)
		}
	}

	if catalog != nil {
		if _, err := catalog.UpsertDirectory(directory); err != nil {
			fmt.Println("Error syncing SQLite directory:", err)
		}
		for _, folder := range filteredFolders {
			if _, err := catalog.UpsertDirectory(filepath.Join(directory, folder.Name)); err != nil {
				fmt.Println("Error syncing SQLite subdirectory:", err)
			}
		}
	}

	for _, folder := range filteredFolders {
		IndexFolder(filepath.Join(directory, folder.Name), bcopy, depth+1, ignored, json, details, music, globalSearch, catalog, writeFiles) // Recursive call.
	}

	// Get list of files and filter ignored ones.
	fileList, _ := GetFiles(directory)
	var filteredFiles []File
	for _, file := range fileList {
		filePath := filepath.Join(directory, file.Name)
		if !IsIgnored(file.Name, ignored) && !shouldSkipCatalogPath(catalog, filePath) {
			filteredFiles = append(filteredFiles, file)
		}
	}

	if catalog != nil {
		if err := catalog.UpsertFiles(directory, filteredFiles); err != nil {
			fmt.Println("Error syncing SQLite files:", err)
		}
	}

	if !writeFiles {
		return
	}

	musicMetadata, err := ResolveExternalLinks(directory, music, catalog)
	if err != nil {
		fmt.Println("Error reading external links:", err)
	}

	// Modify HTML with directory data.
	boilerplate = WriteFolderName(directory, boilerplate)
	jsData := RemoveLastCharacter(WriteFolderJSON(filteredFolders, details) + WriteFileJSON(filteredFiles, details, musicMetadata))
	boilerplate = strings.ReplaceAll(boilerplate, "[data]", fmt.Sprintf("const d = [%s];", jsData))

	// Optionally write data.json file.
	if json {
		jsonPath := filepath.Join(directory, "data.json")
		err = os.WriteFile(jsonPath, []byte(fmt.Sprintf("[%s]", jsData)), 0644)
		if err != nil {
			fmt.Println("Error writing file:", err)
		}
	}

	// Write index.html.
	indexPath := filepath.Join(directory, "index.html")
	err = os.WriteFile(indexPath, []byte(boilerplate), 0644)
	if err != nil {
		fmt.Println("Error writing file:", err)
	}
}

func shouldSkipCatalogPath(catalog *Catalog, path string) bool {
	if catalog == nil {
		return false
	}

	relPath, err := catalog.relativePath(path)
	if err != nil {
		return false
	}

	return catalog.IsCatalogFile(relPath)
}

// WriteSearchPage fills the [data] placeholder in htmlTemplate with the SQLite-backed
// search index and writes search.html to the root of the traced directory.
func WriteSearchPage(htmlTemplate string, catalog *Catalog) error {
	if catalog == nil {
		return fmt.Errorf("sqlite catalog is required for global search")
	}

	entries, err := catalog.GetSearchEntries()
	if err != nil {
		return err
	}

	jsData := RemoveLastCharacter(WriteSearchEntryJSON(entries))
	content := strings.ReplaceAll(htmlTemplate, "[data]", fmt.Sprintf("const sd = [%s];", jsData))
	err = os.WriteFile("./search.html", []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("write search.html: %w", err)
	}

	return nil
}
