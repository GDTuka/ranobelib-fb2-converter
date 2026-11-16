package fb2

import (
	"encoding/base64"
	"encoding/xml"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestBuildFB2CreatesValidXML(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("image"))
	result, err := Build("Book & Story", []Section{{
		Title: "Chapter <1>",
		Elements: []Element{
			{Text: "First & second"},
			{ImageRef: "img_1"},
		},
	}}, []Image{{ID: "img_1", MimeType: "image/png", Data: imageData}})
	if err != nil {
		t.Fatalf("BuildFB2() error = %v", err)
	}

	decoder := xml.NewDecoder(strings.NewReader(result))
	for {
		if _, err := decoder.Token(); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("generated FB2 is not valid XML: %v\n%s", err, result)
		}
	}
	for _, want := range []string{"<genre>prose_contemporary</genre>", "<book-title>Book &amp; Story</book-title>", "<lang>ru</lang>", "<document-info>", "<title><p>Chapter &lt;1&gt;</p></title>", `<image l:href="#img_1"/>`, `<binary content-type="image/png" id="img_1">`} {
		if !strings.Contains(result, want) {
			t.Errorf("generated FB2 does not contain %q", want)
		}
	}
}

func TestGeneratedBookFile(t *testing.T) {
	path := os.Getenv("FB2_INTEGRATION_FILE")
	if path == "" {
		t.Skip("FB2_INTEGRATION_FILE is not set")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var sections, paragraphs int
	var title string
	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("generated FB2 is not valid XML: %v", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "section":
			sections++
		case "p":
			paragraphs++
		case "book-title":
			if err := decoder.DecodeElement(&title, &start); err != nil {
				t.Fatal(err)
			}
		}
	}
	if title == "" || sections == 0 || paragraphs == 0 {
		t.Fatalf("incomplete FB2: title=%q sections=%d paragraphs=%d", title, sections, paragraphs)
	}
	if expected := os.Getenv("FB2_EXPECTED_SECTIONS"); expected != "" {
		count, err := strconv.Atoi(expected)
		if err != nil {
			t.Fatalf("invalid FB2_EXPECTED_SECTIONS: %v", err)
		}
		if sections != count {
			t.Fatalf("sections=%d, expected %d", sections, count)
		}
	}
	t.Logf("validated title=%q sections=%d paragraphs=%d", title, sections, paragraphs)
}

func TestBuildFB2RejectsInvalidImage(t *testing.T) {
	_, err := Build("Book", nil, []Image{{ID: "img_1", MimeType: "image/png", Data: "not-base64"}})
	if err == nil || !strings.Contains(err.Error(), "invalid base64") {
		t.Fatalf("BuildFB2() error = %v", err)
	}
}

func TestBuildFB2RemovesInvalidXMLCharacters(t *testing.T) {
	result, err := Build("Book\x00", []Section{{Elements: []Element{{Text: "Before\x01After"}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(result))
	for {
		if _, err := decoder.Token(); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("generated FB2 is not valid XML: %v", err)
		}
	}
	if strings.ContainsAny(result, "\x00\x01") || !strings.Contains(result, "BeforeAfter") {
		t.Fatalf("invalid XML characters were not removed: %q", result)
	}
}
