package main

import (
	"fmt"
	"log"

	"github.com/justinbornais/go-directory-tracer/utilities"
)

func CheckError(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	files, err := utilities.GetFiles(".")
	CheckError(err)
	folders, err := utilities.GetFolders(".")
	CheckError(err)

	for _, file := range files {
		fmt.Printf("%s\t%s\t%s\t%s\n", file.Name, file.Extension, file.Modified, file.Size)
	}
	for _, folder := range folders {
		fmt.Printf("%s\t%s\t%s\n", folder.Name, folder.Modified, folder.Size)
	}

	fmt.Println("Done.")
}
