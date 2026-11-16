package fb2

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"strings"
	"time"
)

type Image struct {
	ID       string
	Data     string
	MimeType string
}

type Element struct {
	Text     string
	ImageRef string
}

type Section struct {
	Title    string
	Elements []Element
}

func Build(bookName string, sections []Section, images []Image) (string, error) {
	if strings.TrimSpace(bookName) == "" {
		return "", fmt.Errorf("book name is required")
	}
	for _, image := range images {
		if image.ID == "" || image.MimeType == "" {
			return "", fmt.Errorf("image id and MIME type are required")
		}
		if _, err := base64.StdEncoding.DecodeString(image.Data); err != nil {
			return "", fmt.Errorf("image %q contains invalid base64: %w", image.ID, err)
		}
	}

	var fb2 bytes.Buffer
	documentID := fmt.Sprintf("ranobelib-fb2-%x", sha256.Sum256([]byte(bookName)))
	date := time.Now().UTC().Format("2006-01-02")
	fb2.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	fb2.WriteString(`<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0" xmlns:l="http://www.w3.org/1999/xlink">`)
	fb2.WriteString(`<description><title-info><genre>prose_contemporary</genre><author><nickname>RanobeLib</nickname></author><book-title>`)
	fb2.WriteString(escapeXMLText(bookName))
	fb2.WriteString(`</book-title><lang>ru</lang></title-info><document-info><author><nickname>ranobelib-fb2-converter</nickname></author><program-used>ranobelib-fb2-converter</program-used><date value="`)
	fb2.WriteString(date)
	fb2.WriteString(`">`)
	fb2.WriteString(date)
	fb2.WriteString(`</date><id>`)
	fb2.WriteString(documentID)
	fb2.WriteString(`</id><version>1.0</version></document-info></description><body>`)
	for _, section := range sections {
		fb2.WriteString(`<section>`)
		if section.Title != "" {
			fb2.WriteString(`<title><p>`)
			fb2.WriteString(escapeXMLText(section.Title))
			fb2.WriteString(`</p></title>`)
		}
		for _, element := range section.Elements {
			switch {
			case element.ImageRef != "":
				fmt.Fprintf(&fb2, `<image l:href="#%s"/>`, escapeXMLText(element.ImageRef))
			case strings.TrimSpace(element.Text) != "":
				fb2.WriteString(`<p>`)
				fb2.WriteString(escapeXMLText(strings.TrimSpace(element.Text)))
				fb2.WriteString(`</p>`)
			}
		}
		fb2.WriteString(`</section>`)
	}
	fb2.WriteString(`</body>`)
	for _, image := range images {
		fmt.Fprintf(&fb2, `<binary content-type="%s" id="%s">%s</binary>`,
			escapeXMLText(image.MimeType), escapeXMLText(image.ID), image.Data)
	}
	fb2.WriteString(`</FictionBook>`)
	return fb2.String(), nil
}

func escapeXMLText(value string) string {
	value = strings.Map(func(char rune) rune {
		if char == '\t' || char == '\n' || char == '\r' ||
			(char >= 0x20 && char <= 0xD7FF) ||
			(char >= 0xE000 && char <= 0xFFFD) ||
			(char >= 0x10000 && char <= 0x10FFFF) {
			return char
		}
		return -1
	}, value)
	return html.EscapeString(value)
}
