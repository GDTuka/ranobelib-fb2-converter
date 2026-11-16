package converter

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ranobelib.parser/internal/config"
)

func TestAPIConverterConvert(t *testing.T) {
	pngData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/book/chapters":
			fmt.Fprint(w, `{"data":[{"volume":"1","number":"0","branches":[{"branch_id":42}]}]}`)
		case "/book/chapter":
			if got := r.URL.Query().Get("branch_id"); got != "42" {
				t.Errorf("branch_id = %q", got)
			}
			fmt.Fprint(w, `{"data":{"name":"Пролог","content":{"content":[{"type":"paragraph","content":[{"type":"text","text":"Привет, "},{"type":"text","text":"мир"},{"type":"text","text":"!"},{"type":"image","attrs":{"images":[{"image":"cover"}]}},{"type":"text","text":"После"}]}]},"attachments":[{"name":"cover","url":"/cover.png"}]}}`)
		case "/cover.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outputDir := t.TempDir()
	converter := &APIConverter{httpClient: server.Client(), imageBaseURL: server.URL}
	path, err := converter.Convert(context.Background(), config.Config{
		BookName: "Тестовая книга", URL: server.URL + "/book", APIURL: server.URL + "/book", OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(content, new(any)); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
	for _, want := range []string{"Тестовая книга", "Том 1. Глава 0 — Пролог", "<p>Привет, мир!</p>", `l:href="#img_1"`, "<p>После</p>", `id="img_1"`} {
		if !strings.Contains(string(content), want) {
			t.Errorf("output does not contain %q", want)
		}
	}
}

func TestAPIConverterRejectsRedirectToUntrustedHost(t *testing.T) {
	targetReached := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetReached = true
	}))
	defer target.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(target.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:"+strconv.Itoa(mustPort(t, port)), http.StatusFound)
	}))
	defer server.Close()

	converter := NewAPI(server.Client())
	_, err = converter.fetchChapters(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "redirect host") {
		t.Fatalf("fetchChapters() error = %v", err)
	}
	if targetReached {
		t.Fatal("redirect target was reached")
	}
}

func mustPort(t *testing.T, value string) int {
	t.Helper()
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestAPIConverterPreservesExistingFileOnChapterError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/book/chapters" {
			fmt.Fprint(w, `{"data":[{"volume":"1","number":"1","branches":[{"branch_id":7}]}]}`)
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "Test Book.fb2")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	converter := &APIConverter{httpClient: server.Client(), imageBaseURL: server.URL}
	_, err := converter.Convert(context.Background(), config.Config{
		BookName: "Test Book", URL: server.URL + "/book", APIURL: server.URL + "/book", OutputDir: outputDir,
	})
	if err == nil || !strings.Contains(err.Error(), "503 Service Unavailable") {
		t.Fatalf("Convert() error = %v", err)
	}
	content, readErr := os.ReadFile(outputPath)
	if readErr != nil || string(content) != "existing" {
		t.Fatalf("existing output changed: content=%q error=%v", content, readErr)
	}
}
