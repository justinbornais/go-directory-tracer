package utilities

import (
	"bufio"
	"os"
	"strings"
)

// Reads the .fileignore file line-by-line into a string slice.
func ReadFileIgnore(filename string) ([]string, error) {
	var lines []string
	lines = append(lines, "tracer")
	lines = append(lines, "tracer.exe")

	file, err := os.Open(filename)
	if err != nil {
		// If file does not exist, return an empty slice.
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	// Read file line-by-line.
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Check for errors while scanning.
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func IsIgnored(name string, ignored []string) bool {
	for _, ignore := range ignored {
		if name == ignore || strings.HasPrefix(name, ".") {
			return true
		}
	}
	return false
}
