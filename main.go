package main

import (
	"bytes"
	"context"
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
    Content          *string    `json:"content"`
}
// Define your Response and Item structs here

type Config struct {
    BookaName string `json:"bookaName"`
    Url      string `json:"url"`
    ApiUrl   string `json:"apiUrl"`
    OutputDir string `json:"outputDir"`
}

func main() {

    configFile , err := os.ReadFile("config.json")

    if err != nil {
        panic(err)
    }

    var config Config

    err = json.Unmarshal(configFile, &config)

    if err != nil {
        panic(err)
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
    
    // Unmarshal the JSON data into the response variable
    err = json.Unmarshal(decodedBody, &response)

    if err != nil {
        panic(err)
    }

    contentString  := ""

    // Access the Data
    for _, item := range response.Data {
	
        modifiedUrl := changeVolumeChapter(uiRequestUrl + "/read/v23/c19?bid&ui=5260317", item.Volume, item.Number)


        ctx, cancel := chromedp.NewContext(context.Background())
        defer cancel()
    
        fmt.Println(modifiedUrl)

        // Navigate to the page and wait for it to load
        var html string
        err = chromedp.Run(ctx,
            chromedp.Navigate(modifiedUrl),
            chromedp.OuterHTML("html", &html),
        )
    
        if err != nil {
            log.Fatal(err)
        }
       
        // Find the div with class "text-content"
        re := regexp.MustCompile(`<div\s+class="text-content"[^>]*>([\s\S]*?)</div>`)
        matches := re.FindStringSubmatch(html)
    
         
        if len(matches) > 1 {
            divContent := matches[1]
            
            // Find all child elements
            childRe := regexp.MustCompile(`<(\w+)[^>]*>([^<]*)</\w+>`)
            childMatches := childRe.FindAllStringSubmatch(divContent, -1)
    
            for _, child := range childMatches {
                if len(child) > 2 {
                    contentString += fmt.Sprintf("%s\n\n", strings.TrimSpace(child[0]))
                }
            }
        } else {
            fmt.Println("Div with class 'text-content' not found")
        }

		time.Sleep(1 * time.Second)

    }

    contentFB2, err := convertHTMLToFB2(contentString)
    if err != nil {
        log.Fatal(err)
    }
    
    // Define the filename
    filename := config.BookaName + ".fb2"
    
    path := filepath.Join(config.OutputDir, filename)

    // Write the FB2 content to the file in the current directory
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

func convertHTMLToFB2(htmlContent string) (string, error) {
    doc, err := html.Parse(strings.NewReader(htmlContent))
    if err != nil {
        return "", err
    }

    var fb2 bytes.Buffer
    fb2.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0">
<body>`)

    var f func(*html.Node)
    f = func(n *html.Node) {
        if n.Type == html.ElementNode {
            switch n.Data {
            case "h1":
                fb2.WriteString("<title>")
                for c := n.FirstChild; c != nil; c = c.NextSibling {
                    if c.Type == html.TextNode {
                        fb2.WriteString(c.Data)
                    }
                }
                fb2.WriteString("</title>")
            case "p":
                fb2.WriteString("<p>")
                for c := n.FirstChild; c != nil; c = c.NextSibling {
                    if c.Type == html.TextNode {
                        fb2.WriteString(c.Data)
                    }
                }
                fb2.WriteString("</p>")
            }
        }
        for c := n.FirstChild; c != nil; c = c.NextSibling {
            f(c)
        }
    }
    f(doc)

    fb2.WriteString("</body></FictionBook>")
    return fb2.String(), nil
}