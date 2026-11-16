package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
)

type Config struct {
	BookName  string `json:"bookName"`
	URL       string `json:"url"`
	APIURL    string `json:"apiUrl"`
	OutputDir string `json:"outputDir"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var config Config
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return Config{}, fmt.Errorf("parse config %q: trailing data: %w", path, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return config, nil
}

func (c Config) Validate() error {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(c.BookName) == "" {
		missing = append(missing, "bookName")
	}
	if strings.TrimSpace(c.URL) == "" {
		missing = append(missing, "url")
	}
	if strings.TrimSpace(c.APIURL) == "" {
		missing = append(missing, "apiUrl")
	}
	if strings.TrimSpace(c.OutputDir) == "" {
		missing = append(missing, "outputDir")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	if strings.ContainsAny(c.BookName, `/\\<>:"|?*`) || c.BookName == "." || c.BookName == ".." {
		return fmt.Errorf("bookName must not contain path separators or reserved filename characters")
	}

	parsedURLs := make(map[string]*url.URL, 2)
	for field, value := range map[string]string{"url": c.URL, "apiUrl": c.APIURL} {
		parsed, err := url.ParseRequestURI(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("%s must be an absolute HTTP URL", field)
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return fmt.Errorf("%s must not contain credentials, query, or fragment", field)
		}
		parsedURLs[field] = parsed
	}
	if pageBook := path.Base(parsedURLs["url"].Path); pageBook != "." {
		if apiBook := path.Base(parsedURLs["apiUrl"].Path); apiBook != pageBook {
			return fmt.Errorf("url and apiUrl must identify the same book")
		}
	}
	return nil
}
