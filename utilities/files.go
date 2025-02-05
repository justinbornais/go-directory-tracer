package utilities

import (
	"os"
	"path/filepath"
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
