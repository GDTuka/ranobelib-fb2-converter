package converter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"ranobelib.parser/internal/fb2"
)

const (
	maxChapterCount = 5000
	maxImageBytes   = 25 << 20
	maxTotalImages  = 250 << 20
	maxTotalText    = 100 << 20
)

type chapterListResponse struct {
	Data []chapterItem `json:"data"`
}

type chapterItem struct {
	Volume   string `json:"volume"`
	Number   string `json:"number"`
	Branches []struct {
		BranchID int `json:"branch_id"`
	} `json:"branches"`
}

func (c *APIConverter) fetchChapters(ctx context.Context, apiURL string) ([]chapterItem, error) {
	endpoint, err := appendURLPath(apiURL, "chapters")
	if err != nil {
		return nil, fmt.Errorf("build chapters URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create chapters request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch chapters: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil, fmt.Errorf("fetch chapters: unexpected HTTP status %s", response.Status)
	}

	var result chapterListResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 10<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode chapters response: %w", err)
	}
	return result.Data, nil
}

func (c *APIConverter) downloadImage(ctx context.Context, imageURL string, number int) (fb2.Image, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return fb2.Image{}, fmt.Errorf("create image request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fb2.Image{}, fmt.Errorf("download image %s: %w", imageURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return fb2.Image{}, fmt.Errorf("download image %s: unexpected HTTP status %s", imageURL, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxImageBytes+1))
	if err != nil {
		return fb2.Image{}, fmt.Errorf("read image %s: %w", imageURL, err)
	}
	if len(data) > maxImageBytes {
		return fb2.Image{}, fmt.Errorf("download image %s: size exceeds %d bytes", imageURL, maxImageBytes)
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return fb2.Image{}, fmt.Errorf("download image %s: unsupported content type %q", imageURL, mimeType)
	}
	if mimeType != "image/webp" {
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return fb2.Image{}, fmt.Errorf("decode image %s: %w", imageURL, err)
		}
	}
	return fb2.Image{ID: fmt.Sprintf("img_%d", number), Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType}, nil
}

func resolveURL(baseURL, ref string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse page URL: %w", err)
	}
	reference, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parse image URL: %w", err)
	}
	resolved := base.ResolveReference(reference)
	if (resolved.Scheme != "http" && resolved.Scheme != "https") || resolved.Hostname() == "" || resolved.User != nil {
		return "", fmt.Errorf("image URL must be an absolute HTTP URL without credentials")
	}
	if !sameHost(base.Hostname(), resolved.Hostname()) && !trustedImageHost(resolved.Hostname()) {
		return "", fmt.Errorf("image host %q is not allowed", resolved.Hostname())
	}
	return resolved.String(), nil
}

func appendURLPath(rawURL, segment string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + segment
	return parsed.String(), nil
}

func sameHost(left, right string) bool {
	return strings.EqualFold(strings.TrimSuffix(left, "."), strings.TrimSuffix(right, "."))
}

func trustedImageHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range []string{"ranobelib.me", "mangalib.me", "novelslib.me", "imglib.info", "imgslib.link", "cdnlibs.org", "lib.social"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func writeFileAtomic(outputDir, filename string, data []byte) (string, error) {
	if filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\\`) {
		return "", fmt.Errorf("invalid output filename %q", filename)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(outputDir, ".fb2-*")
	if err != nil {
		return "", fmt.Errorf("create temporary FB2: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		cleanup()
		return "", fmt.Errorf("write temporary FB2: %w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close temporary FB2: %w", err)
	}
	outputPath := filepath.Join(outputDir, filename)
	if err := replaceFile(temporaryPath, outputPath); err != nil {
		cleanup()
		return "", fmt.Errorf("replace FB2 file: %w", err)
	}
	return outputPath, nil
}
