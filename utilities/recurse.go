package utilities

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func IndexFolder(directory, boilerplate string, depth int, ignored []string) {
	fmt.Println("Indexing directory:", directory)

	bcopy := boilerplate // Used for recursive call.
	base := strings.Repeat("../", depth)
	boilerplate = strings.ReplaceAll(boilerplate, "[base]", base)

	// Get list of folders and filter ignored ones.
	folders, _ := GetFolders(directory)
	var filteredFolders []Folder
	for _, folder := range folders {
		if !IsIgnored(folder.Name, ignored) {
			filteredFolders = append(filteredFolders, folder)
			IndexFolder(filepath.Join(directory, folder.Name), bcopy, depth+1, ignored) // Recursive call.
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

	// Modify HTML with directory data.
	boilerplate = WriteFolderName(directory, boilerplate)
	jsData := WriteFolderJSON(filteredFolders) + WriteFileJSON(filteredFiles)
	boilerplate = strings.ReplaceAll(boilerplate, "[data]", fmt.Sprintf("const d = [%s];", jsData))

	// Write index.html.
	indexPath := filepath.Join(directory, "index.html")
	err := os.WriteFile(indexPath, []byte(boilerplate), 0644)
	if err != nil {
		fmt.Println("Error writing file:", err)
	}
}
