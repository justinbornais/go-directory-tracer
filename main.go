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
	json := flag.Bool("json", false, "Specifies if directory contents should also be saved to a 'data.json' file for each directory")
	details := flag.Bool("details", false, "Specifies if JSON objects should also include modified dates and file sizes")
	android := flag.Bool("android", false, "Specifies if the Google Docs viewer should be used when displaying PDFs")
	music := flag.Bool("music", false, "Specifies if metadata.json should be used to add audio URLs to data.json files")
	globalSearch := flag.Bool("global-search", false, "Generate a root-level search.html with a cross-directory search index")
	flag.Parse()

	css := utilities.GetCSS(StaticFiles)
	js := utilities.GetJS(StaticFiles, *audio, *android)
	ignored, err := utilities.ReadFileIgnore("./.fileignore")
	utilities.CheckError(err)

	html := utilities.GenerateBoilerplateHTML(*title, css, js)

	var entries []utilities.SearchEntry
	utilities.IndexFolder(".", html, 0, ignored, *json, *details, *music, *globalSearch, &entries)

	if *globalSearch {
		searchJS := utilities.GetSearchJS(StaticFiles)
		searchHTML := utilities.GenerateSearchHTML(*title, css, searchJS)
		utilities.WriteSearchPage(searchHTML, entries)
	}

	fmt.Println("Done.")
}
