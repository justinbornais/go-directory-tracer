package utilities

import (
	"fmt"
	"strings"
)

func WriteFolderJSON(folders []Folder) string {
	var data strings.Builder
	for _, f := range folders {
		data.WriteString(fmt.Sprintf(`{"n":"%s","t":"d", "m":"%s", "s":"%s"},`, f.Name, f.Modified, f.Size))
	}
	return data.String()
}

func WriteFileJSON(files []File) string {
	var data strings.Builder
	for _, f := range files {
		if f.Name != "index.html" {
			data.WriteString(fmt.Sprintf(`{"n":"%s","t":"f", "m":"%s", "s":"%s"},`, f.Name, f.Modified, f.Size))
		}
	}
	return data.String()
}
