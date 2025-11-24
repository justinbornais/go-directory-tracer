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
	data, err := file.ReadFile("static/default.css")
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

func GetJS(file embed.FS, audio, android bool) string {
	data, err := file.ReadFile("static/default.js")
	CheckError(err)

	strData := string(data)

	re := regexp.MustCompile(`/\*.*?\*/`)
	strData = re.ReplaceAllString(strData, "")
	strData = strings.ReplaceAll(strData, "\n", "")
	strData = strings.ReplaceAll(strData, "\r", "")
	strData = strings.ReplaceAll(strData, "\t", "")
	strData = strings.Join(strings.Fields(strData), " ") // Remove extra spaces.
	if audio {
		strData = strings.ReplaceAll(strData, "[audio_embed]", "true")
	} else {
		strData = strings.ReplaceAll(strData, "[audio_embed]", "false")
	}
	if android {
		strData = strings.ReplaceAll(strData, "[android_pdf]", "true")
	} else {
		strData = strings.ReplaceAll(strData, "[android_pdf]", "false")
	}

	return strData
}
