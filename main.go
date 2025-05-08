package main

import (
	"embed"
	"flag"
	"fmt"

	"github.com/justinbornais/go-directory-tracer/utilities"
)

//go:embed static
var StaticFiles embed.FS

func main() {
	// Get from command line.
	title := flag.String("title", "Sample Website", "Specify the title of the website")
	audio := flag.Bool("audio-embed", false, "Specifies if audio files should be directly embedded inside the HTML")
	flag.Parse()

	css := utilities.GetCSS(StaticFiles)
	js := utilities.GetJS(StaticFiles, *audio)
	ignored, err := utilities.ReadFileIgnore("./.fileignore")
	utilities.CheckError(err)

	html := utilities.GenerateBoilerplateHTML(*title, css, js)

	utilities.IndexFolder(".", html, 0, ignored)

	fmt.Println("Done.")
}
