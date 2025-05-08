package main

import (
	"embed"
	"flag"
	"fmt"

	"github.com/justinbornais/go-directory-tracer/utilities"
)

//go:embed static/style.css
var CSSData embed.FS

//go:embed static/script.js
var JSData embed.FS

func main() {
	// Get from command line.
	title := flag.String("title", "Sample Website", "Specify the title of the website")
	flag.Parse()

	css := utilities.GetCSS(CSSData)
	js := utilities.GetJS(JSData)
	ignored, err := utilities.ReadFileIgnore("./.fileignore")
	utilities.CheckError(err)

	html := utilities.GenerateBoilerplateHTML(*title, css, js)

	utilities.IndexFolder(".", html, 0, ignored)

	fmt.Println("Done.")
}
