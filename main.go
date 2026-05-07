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
	sqlitePath := flag.String("sqlite", "", "Optional path to a SQLite catalog relative to the traced root")
	sqliteOnly := flag.Bool("sqlite-only", false, "Only sync the SQLite catalog and do not generate index.html, data.json, or search.html files")
	sqlitePruneMissing := flag.Bool("sqlite-prune-missing", false, "Remove SQLite rows for files and directories that no longer exist on disk")
	sqliteTimestamps := flag.Bool("sqlite-timestamps", false, "Include created_at and updated_at Unix timestamp columns in the SQLite catalog")
	globalSearch := flag.Bool("global-search", false, "Generate a root-level search.html with a cross-directory search index")
	flag.Parse()

	css := utilities.GetCSS(StaticFiles)
	js := utilities.GetJS(StaticFiles, *audio, *android)
	ignored, err := utilities.ReadFileIgnore("./.fileignore")
	utilities.CheckError(err)

	var catalog *utilities.Catalog
	if *sqlitePath != "" {
		catalog, err = utilities.OpenCatalog(".", *sqlitePath, utilities.CatalogOptions{IncludeTimestamps: *sqliteTimestamps})
		utilities.CheckError(err)
		defer func() {
			utilities.CheckError(catalog.Close())
		}()
	}
	if *globalSearch && catalog == nil {
		utilities.CheckError(fmt.Errorf("--global-search requires --sqlite"))
	}
	if *sqliteOnly && catalog == nil {
		utilities.CheckError(fmt.Errorf("--sqlite-only requires --sqlite"))
	}
	if *sqlitePruneMissing && catalog == nil {
		utilities.CheckError(fmt.Errorf("--sqlite-prune-missing requires --sqlite"))
	}

	html := utilities.GenerateBoilerplateHTML(*title, css, js)

	if catalog != nil {
		utilities.CheckError(catalog.WithSyncTransaction(func() error {
			utilities.IndexFolder(".", html, 0, ignored, *json, *details, *music, *globalSearch, catalog, !*sqliteOnly)
			if *sqlitePruneMissing {
				if err := catalog.PruneMissingEntries(); err != nil {
					return err
				}
			}
			return nil
		}))
	} else {
		utilities.IndexFolder(".", html, 0, ignored, *json, *details, *music, *globalSearch, catalog, !*sqliteOnly)
	}

	if *globalSearch && !*sqliteOnly {
		searchJS := utilities.GetSearchJS(StaticFiles)
		searchHTML := utilities.GenerateSearchHTML(*title, css, searchJS)
		utilities.CheckError(utilities.WriteSearchPage(searchHTML, catalog))
	}
	if catalog != nil {
		utilities.CheckError(catalog.Vacuum())
	}

	fmt.Println("Done.")
}
