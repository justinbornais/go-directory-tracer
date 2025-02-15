package utilities

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func GetFolders(directory string) ([]Folder, error) {
	var folders []Folder

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			fullPath := filepath.Join(directory, entry.Name())

			// Get folder info.
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			size := GetFolderSize(fullPath)
			folders = append(folders, Folder{
				Name:     entry.Name(),
				Size:     HumanReadableFileSize(size),
				Modified: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}

	return folders, nil
}

func GetFiles(directory string) ([]File, error) {
	var files []File

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() { // Only process files.
			fullPath := filepath.Join(directory, entry.Name())

			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			ext := filepath.Ext(entry.Name())
			files = append(files, File{
				Name:      entry.Name(),
				Size:      HumanReadableFileSize(info.Size()),
				Modified:  info.ModTime().Format("2006-01-02 15:04:05"),
				Extension: ext,
			})
		}
	}

	return files, nil
}

// Helper function to read file as string.
func ReadFileToString(filePath string) (string, error) {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	fileContents := string(data)

	// Remove block comments and unnecessary whitespace.
	re := regexp.MustCompile(`/\*.*?\*/`)
	fileContents = re.ReplaceAllString(fileContents, "")
	fileContents = strings.ReplaceAll(fileContents, "\n", "")
	fileContents = strings.ReplaceAll(fileContents, "\r", "")
	fileContents = strings.ReplaceAll(fileContents, "\t", "")

	// Remove extra spaces.
	fileContents = strings.Join(strings.Fields(fileContents), " ")

	return fileContents, nil
}
