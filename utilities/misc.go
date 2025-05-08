package utilities

import (
	"embed"
	"log"
	"regexp"
	"strings"
)

func CheckError(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func GetCSS(file embed.FS) string {
	data, err := file.ReadFile("static/style.css")
	CheckError(err)

	strData := string(data)

	re := regexp.MustCompile(`/\*.*?\*/`)
	strData = re.ReplaceAllString(strData, "")
	strData = strings.ReplaceAll(strData, "\n", "")
	strData = strings.ReplaceAll(strData, "\r", "")
	strData = strings.ReplaceAll(strData, "\t", "")
	strData = strings.Join(strings.Fields(strData), " ") // Remove extra spaces.
	return strData
}

func GetJS(file embed.FS) string {
	data, err := file.ReadFile("static/script.js")
	CheckError(err)

	strData := string(data)

	re := regexp.MustCompile(`/\*.*?\*/`)
	strData = re.ReplaceAllString(strData, "")
	strData = strings.ReplaceAll(strData, "\n", "")
	strData = strings.ReplaceAll(strData, "\r", "")
	strData = strings.ReplaceAll(strData, "\t", "")
	strData = strings.Join(strings.Fields(strData), " ") // Remove extra spaces.
	return strData
}
