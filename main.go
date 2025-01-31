package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

type Response struct {
	Data []Item `json:"data"`
}

type Item struct {
	ID     int    `json:"id"`
	Volume string `json:"volume"`
	Number string `json:"number"`
}

type ContentRes struct {
	Data ChapterData `json:"data"`
}

type ChapterData struct {
	Content *string `json:"content"`
}

type Config struct {
	BookName  string `json:"bookName"`
	Url       string `json:"url"`
	ApiUrl    string `json:"apiUrl"`
	OutputDir string `json:"outputDir"`
}

type FB2Image struct {
	ID       string
	Data     string
	MimeType string
}

func downloadImage(url string) (FB2Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return FB2Image{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return FB2Image{}, err
	}

	id := fmt.Sprintf("img_%d", time.Now().UnixNano())
	encodedData := base64.StdEncoding.EncodeToString(data)

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		// Default to image/jpeg if no content type is provided
		mimeType = "image/jpeg"
	}

	return FB2Image{
		ID:       id,
		Data:     encodedData,
		MimeType: mimeType,
	}, nil
}

func main() {
	handleDownloadAll(true)
}

func handleDownloadAll(useConfig bool) {
	var jsonParam string
	var config Config
	if useConfig {

		file, err := os.ReadFile("config.json")
		if err != nil {
			fmt.Println("Error reading config file:", err)
			os.Exit(1)
		}
		err = json.Unmarshal(file, &config)
		if err != nil {
			fmt.Println("Error parsing JSON:", err)
			return
		}

			fmt.Println("Using config")
		} else {

			if len(os.Args) < 2 {
				fmt.Println("No JSON parameter provided")
				return
			}
		
			jsonParam = os.Args[1]
			err := json.Unmarshal([]byte(jsonParam), &config)
			if err != nil {
				fmt.Println("Error parsing JSON:", err)
				return
			}
		
		}

	uiRequestUrl := config.Url
	requestUrl := config.ApiUrl
	params := "/chapters"

	// Create a new request
	resp, err := http.Get(requestUrl + params)
	if err != nil {
		panic(err)
	}

	fmt.Println("Get request successful")

	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	fmt.Println("Read body")
	if err != nil {
		panic(err)
	}

	// Convert to UTF-8
	bodyReader := bytes.NewReader(body)
	e, _, _ := charset.DetermineEncoding(body, resp.Header.Get("Content-Type"))
	utf8Reader := transform.NewReader(bodyReader, e.NewDecoder())
	decodedBody, err := io.ReadAll(utf8Reader)
	if err != nil {
		panic(err)
	}
	fmt.Println("Decode body")

	var response Response

	err = json.Unmarshal(decodedBody, &response)
	if err != nil {
		panic(err)
	}

	contentString := ""
	var allImages []FB2Image

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*30*time.Second)
	defer cancel()

	// Create Chrome instance
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	ctx, cancel = chromedp.NewContext(allocCtx)
	defer cancel()

	// Access the Data
	for _, item := range response.Data {
		modifiedUrl := changeVolumeChapter(uiRequestUrl+"/read/v23/c19?ui=5260317", item.Volume, item.Number)
		fmt.Println(modifiedUrl)

		var html string
		err = chromedp.Run(ctx,
			chromedp.Navigate(modifiedUrl),
			// Wait for body to be present
			chromedp.WaitReady("body", chromedp.ByQuery),
			// Wait for text-content div to be present
			chromedp.WaitVisible(".text-content", chromedp.ByQuery),
			// Additional wait to ensure JavaScript loads
			chromedp.Sleep(2*time.Second),
			chromedp.OuterHTML("html", &html),
		)

		if err != nil {
			log.Printf("Error processing URL %s: %v", modifiedUrl, err)
			continue
		}

		headerRe := regexp.MustCompile(`<h1[^>]*>([\s\S]*?)</h1>`)
		headerMathes := headerRe.FindStringSubmatch(html)

		if len(headerMathes) > 1 {
			fmt.Println("H1 found %s", strings.TrimSpace(headerMathes[0]))
			contentString += fmt.Sprintf("%s\n\n", strings.TrimSpace(headerMathes[0]))
		} else {
			fmt.Println("H1 not found")
		}

		// Find the div with class "text-content"
		re := regexp.MustCompile(`<div\s+class="text-content"[^>]*>([\s\S]*?)</div>`)
		matches := re.FindStringSubmatch(html)

		if len(matches) < 1 {
			re := regexp.MustCompile(`<div\s+class="node-doc text-content"[^>]*>([\s\S]*?)</div>`)
			matches = re.FindStringSubmatch(html)
		}

		if len(matches) > 1 {
			divContent := matches[1]

			// First, find and process images within the text-content div
			imgRe := regexp.MustCompile(`<img[^>]+src="([^"]+)"[^>]*>`)
			imgMatches := imgRe.FindAllStringSubmatch(divContent, -1)
			if len(imgMatches) < 1 {
				imgRe = regexp.MustCompile(`<img[^>]+class="_loaded node-image-item"[^>]+src="([^"]+)"[^>]*>`)
				imgMatches = imgRe.FindAllStringSubmatch(divContent, -1)
			}

			// Create a map to store image replacements
			imageReplacements := make(map[string]string)

			for _, match := range imgMatches {
				if len(match) > 1 {
					imgURL := match[1]
					if !strings.HasPrefix(imgURL, "http") {
						// Handle relative URLs
						if !strings.Contains(imgURL, "ranobelib.me") {
							imgURL = "https://ranobelib.me" + imgURL
						} else if strings.HasPrefix(imgURL, "//") {
							imgURL = "https:" + imgURL
						} else {
							imgURL = "https://" + strings.TrimPrefix(imgURL, "/")
						}
					}

					img, err := downloadImage(imgURL)
					if err != nil {
						log.Printf("Error downloading image %s: %v", imgURL, err)
						continue
					}
					allImages = append(allImages, img)

					// Store the replacement for this image
					imageReplacements[match[0]] = fmt.Sprintf(`<image l:href="#%s"/>`, img.ID)
				}
			}

			// Replace images in the content with FB2 image references
			for oldImg, newImg := range imageReplacements {
				divContent = strings.Replace(divContent, oldImg, newImg, -1)
			}

			// Now process all elements including the replaced images
			childRe := regexp.MustCompile(`<(\w+)[^>]*>([^<]*)</\w+>|<image[^>]+/>`)
			childMatches := childRe.FindAllString(divContent, -1)

			for _, child := range childMatches {
				contentString += fmt.Sprintf("%s\n\n", strings.TrimSpace(child))
			}
		} else {
			fmt.Println("Div with class 'text-content' not found")
		}

		time.Sleep(1 * time.Second)
	}

	contentFB2, err := convertHTMLToFB2(contentString, allImages)
	if err != nil {
		log.Fatal(err)
	}

	// Define the filename
	filename := config.BookName + ".fb2"
	path := filepath.Join(config.OutputDir, filename)

	// Write the FB2 content to the file
	err = os.WriteFile(path, []byte(contentFB2), 0644)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("FB2 file successfully created: %s\n", filename)
}

func changeVolumeChapter(url string, newVolume string, newChapter string) string {
	re := regexp.MustCompile(`/v\d+/c\d+`)
	return re.ReplaceAllString(url, fmt.Sprintf("/v%s/c%s", newVolume, newChapter))
}

func convertHTMLToFB2(htmlContent string, images []FB2Image) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var fb2 bytes.Buffer
	fb2.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0" xmlns:l="http://www.w3.org/1999/xlink">
<description>
</description>
<body>
`)

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1":
				fb2.WriteString("<title>")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						fb2.WriteString(html.EscapeString(c.Data))
					}
				}
				fb2.WriteString("</title>")
			case "p":
				fb2.WriteString("<p>")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						fb2.WriteString(html.EscapeString(c.Data))
					}
				}
				fb2.WriteString("</p>")
			case "img":
				for _, attr := range n.Attr {
					if attr.Key == "l:href" {
						// Добавляем символ "#" перед ссылкой на изображение
						fb2.WriteString(fmt.Sprintf(`<image l:href="%s"/>`, attr.Val))
						break
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(doc)
	fb2.WriteString("</body>")

	if len(images) > 0 {
		for _, img := range images {
			fb2.WriteString(fmt.Sprintf(`<binary content-type="%s" id="%s">%s</binary>`,
				img.MimeType, img.ID, img.Data))
		}
	}

	fb2.WriteString("</FictionBook>")
	return fb2.String(), nil
}