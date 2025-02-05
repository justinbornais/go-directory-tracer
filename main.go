package main

import (
	"fmt"

	"github.com/justinbornais/go-directory-tracer/utilities"
)

const (
	TITLE = "Sample Website"
)

func main() {
	css, err := utilities.ReadFileToString("./static/style.css")
	utilities.CheckError(err)
	js, err := utilities.ReadFileToString("./static/script.js")
	utilities.CheckError(err)
	ignored, err := utilities.ReadFileIgnore("./.fileignore")
	utilities.CheckError(err)

	html := utilities.GenerateBoilerplateHTML(TITLE, css, js)

	utilities.IndexFolder(".", html, 0, ignored)

	fmt.Println("Done.")
}
