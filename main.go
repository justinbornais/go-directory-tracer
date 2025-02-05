package main

import (
	"flag"
	"fmt"

	"github.com/justinbornais/go-directory-tracer/utilities"
)

func main() {
	// Get from command line.
	title := flag.String("title", "Sample Website", "Specify the title of the website")
	flag.Parse()

	css := utilities.GetCSS()
	js := utilities.GetJS()
	ignored, err := utilities.ReadFileIgnore("./.fileignore")
	utilities.CheckError(err)

	html := utilities.GenerateBoilerplateHTML(*title, css, js)

	utilities.IndexFolder(".", html, 0, ignored)

	fmt.Println("Done.")
}
