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
    <div id="top"><h1>[parent]<a class="n" href="[base]">%s</a>[dir]</h1>[global-search]
        <input type="search" class="q" id="q" placeholder="Search..." />
    </div>
    <ul id="dl"></ul>
    <script src="https://cdn.jsdelivr.net/npm/fuse.js@6.6.2"></script>
    <script>[data]%s</script>
</body>
</html>`, title, css, title, js)
}

func GenerateSearchHTML(title, css, js string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Search — %s</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width">
    <meta name="robots" content="noindex, nofollow">
    <link rel="icon" href="data:;base64,iVBORw0KGgo=">
    <style>%s#dl{display:flex;flex-direction:column;gap:.35rem;padding-top:.1rem}.sr-group{border:1px solid rgba(128,128,128,.2);border-radius:6px;overflow:hidden;transition:box-shadow .12s}.sr-header{padding:.3rem .6rem;background:rgba(128,128,128,.08);border-bottom:1px solid rgba(128,128,128,.15);font-weight:700;font-size:1.05rem}.sr-item{display:block;padding:.2rem .5rem .2rem 1.5rem;transition:background .12s}.sr-item:hover{background:rgba(128,128,128,.08)}</style>
</head>
<body>
    <div id="top">
        <h1><b><a href="./" class="p">&#8592;</a></b><a class="n" href="./">%s</a> &#8212; Global Search</h1>
        <input type="search" class="q" id="q" placeholder="Search all files and folders..." autofocus />
    </div>
    <div id="dl"></div>
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
