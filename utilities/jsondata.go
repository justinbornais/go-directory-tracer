package utilities

import (
	"fmt"
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

func WriteFileJSON(files []File, details bool) string {
	var data strings.Builder
	for _, f := range files {
		if f.Name == "index.html" {
			continue
		}
		if details {
			data.WriteString(fmt.Sprintf(`{"n":"%s","t":"f", "m":"%s", "s":"%s"},`, f.Name, f.Modified, f.Size))
		} else {
			data.WriteString(fmt.Sprintf(`{"n":"%s","t":"f"},`, f.Name))
		}
	}
	return data.String()
}
