package utilities

import (
	"fmt"
	"strings"
)

func GenerateBoilerplateHTML(title, css, js string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%s</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width">
	<meta name="robots" content="noindex, nofollow">
    <link rel="icon" href="data:;base64,iVBORw0KGgo=">
    <style>%s</style>
</head>
<body>
    <div id="top"><h1>[parent]<a class="n" href="[base]">%s</a>[dir]</h1>
        <input type="text" class="q" id="q" placeholder="Search for a file" />
    </div>
    <ul id="dl"></ul>
    <script src="https://cdn.jsdelivr.net/npm/fuse.js@6.6.2"></script>
    <script>[data]%s</script>
</body>
</html>`, title, css, title, js)
}

// Write the folder name at the top, with links to all subfolders in between.
func WriteFolderName(directory string, boilerplate string) string {
	var title, parent string
	if directory != "." {
		dir := strings.ReplaceAll(directory, "\\", "/")
		parts := strings.Split(dir, "/")
		title = " - "
		for i, part := range parts {
			depth := strings.Repeat("../", len(parts)-i)
			title += fmt.Sprintf(`<a class="n" href="%s%s">%s</a>`, depth, part, part)
			if i != len(parts)-1 {
				title += "/"
			}
		}

		parent = `<b><a href=".." class="p">←</a></b>`
	}
	boilerplate = strings.ReplaceAll(boilerplate, "[dir]", title)
	boilerplate = strings.ReplaceAll(boilerplate, "[parent]", parent)
	return boilerplate
}
