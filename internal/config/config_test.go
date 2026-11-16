package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"bookName":"Book","url":"https://example.test/book","apiUrl":"https://api.example.test/book","outputDir":"` + dir + `"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.BookName != "Book" || config.OutputDir != dir {
		t.Fatalf("Load() = %#v", config)
	}
}

func TestLoadConfigRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "invalid JSON", content: `{`, want: "parse config"},
		{name: "missing field", content: `{"bookName":"Book","url":"https://example.test","apiUrl":"https://api.example.test"}`, want: "outputDir"},
		{name: "invalid URL", content: `{"bookName":"Book","url":"relative","apiUrl":"https://api.example.test","outputDir":"out"}`, want: "absolute HTTP URL"},
		{name: "unknown field", content: `{"bookName":"Book","url":"https://example.test","apiUrl":"https://api.example.test","outputDir":"out","extra":true}`, want: "unknown field"},
		{name: "trailing JSON", content: `{"bookName":"Book","url":"https://example.test","apiUrl":"https://api.example.test","outputDir":"out"} {}`, want: "trailing data"},
		{name: "unsafe filename", content: `{"bookName":"../Book","url":"https://example.test","apiUrl":"https://api.example.test","outputDir":"out"}`, want: "reserved filename"},
		{name: "URL query", content: `{"bookName":"Book","url":"https://example.test?x=1","apiUrl":"https://api.example.test","outputDir":"out"}`, want: "query"},
		{name: "different books", content: `{"bookName":"Book","url":"https://example.test/1--first","apiUrl":"https://api.example.test/2--second","outputDir":"out"}`, want: "same book"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
