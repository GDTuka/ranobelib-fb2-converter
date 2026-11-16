package converter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ranobelib.parser/internal/config"
	"ranobelib.parser/internal/fb2"
)

const (
	defaultImageBaseURL = "https://v1.novelslib.me"
	apiRequestInterval  = 650 * time.Millisecond
)

type APIConverter struct {
	httpClient      *http.Client
	imageBaseURL    string
	requestInterval time.Duration
	lastRequest     time.Time
}

type apiChapterResponse struct {
	Data apiChapter `json:"data"`
}

type apiChapter struct {
	Name        string          `json:"name"`
	Content     apiDocument     `json:"content"`
	Attachments []apiAttachment `json:"attachments"`
}

type apiDocument struct {
	Content []apiNode `json:"content"`
}

type apiNode struct {
	Type    string    `json:"type"`
	Text    string    `json:"text"`
	Content []apiNode `json:"content"`
	Attrs   struct {
		Images []struct {
			Image string `json:"image"`
		} `json:"images"`
	} `json:"attrs"`
}

type apiAttachment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func NewAPI(httpClient *http.Client) *APIConverter {
	if httpClient != nil {
		httpClient = withRetryClient(httpClient)
	}
	return &APIConverter{
		httpClient:      httpClient,
		imageBaseURL:    defaultImageBaseURL,
		requestInterval: apiRequestInterval,
	}
}

func (c *APIConverter) Convert(ctx context.Context, appConfig config.Config) (string, error) {
	if err := appConfig.Validate(); err != nil {
		return "", err
	}
	if c.httpClient == nil {
		return "", fmt.Errorf("HTTP client is required")
	}

	chapters, err := c.fetchChapters(ctx, appConfig.APIURL)
	if err != nil {
		return "", err
	}
	if len(chapters) == 0 {
		return "", fmt.Errorf("RanobeLib returned no chapters")
	}
	if len(chapters) > maxChapterCount {
		return "", fmt.Errorf("RanobeLib returned %d chapters; limit is %d", len(chapters), maxChapterCount)
	}

	sections := make([]fb2.Section, 0, len(chapters))
	images := make([]fb2.Image, 0)
	totalImageBytes := 0
	totalTextBytes := 0
	for _, chapter := range chapters {
		if len(chapter.Branches) == 0 {
			return "", fmt.Errorf("chapter %s/%s has no translation branch", chapter.Volume, chapter.Number)
		}
		chapterData, err := c.fetchChapter(ctx, appConfig.APIURL, chapter.Volume, chapter.Number, chapter.Branches[0].BranchID)
		if err != nil {
			return "", fmt.Errorf("fetch chapter %s/%s: %w", chapter.Volume, chapter.Number, err)
		}
		section, chapterImages, err := c.convertChapter(ctx, chapter, chapterData, len(images))
		if err != nil {
			return "", fmt.Errorf("convert chapter %s/%s: %w", chapter.Volume, chapter.Number, err)
		}
		sections = append(sections, section)
		for _, element := range section.Elements {
			totalTextBytes += len(element.Text)
		}
		if totalTextBytes > maxTotalText {
			return "", fmt.Errorf("chapter text exceeds total limit of %d bytes", maxTotalText)
		}
		images = append(images, chapterImages...)
		for _, image := range chapterImages {
			totalImageBytes += base64.StdEncoding.DecodedLen(len(image.Data))
		}
		if totalImageBytes > maxTotalImages {
			return "", fmt.Errorf("embedded images exceed total limit of %d bytes", maxTotalImages)
		}
	}

	content, err := fb2.Build(appConfig.BookName, sections, images)
	if err != nil {
		return "", fmt.Errorf("build FB2: %w", err)
	}
	return writeFileAtomic(appConfig.OutputDir, appConfig.BookName+".fb2", []byte(content))
}

func (c *APIConverter) fetchChapter(ctx context.Context, apiURL, volume, number string, branchID int) (apiChapter, error) {
	endpoint, err := appendURLPath(apiURL, "chapter")
	if err != nil {
		return apiChapter{}, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return apiChapter{}, err
	}
	query := parsed.Query()
	query.Set("volume", volume)
	query.Set("number", number)
	query.Set("branch_id", strconv.Itoa(branchID))
	parsed.RawQuery = query.Encode()

	if wait := c.requestInterval - time.Since(c.lastRequest); wait > 0 {
		select {
		case <-ctx.Done():
			return apiChapter{}, ctx.Err()
		case <-time.After(wait):
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return apiChapter{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	c.lastRequest = time.Now()
	if err != nil {
		return apiChapter{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return apiChapter{}, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	var result apiChapterResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 20<<20)).Decode(&result); err != nil {
		return apiChapter{}, err
	}
	if len(result.Data.Content.Content) == 0 {
		return apiChapter{}, fmt.Errorf("chapter contains no content")
	}
	return result.Data, nil
}

func (c *APIConverter) convertChapter(ctx context.Context, item chapterItem, chapter apiChapter, imageOffset int) (fb2.Section, []fb2.Image, error) {
	title := fmt.Sprintf("Том %s. Глава %s", item.Volume, item.Number)
	if strings.TrimSpace(chapter.Name) != "" {
		title += " — " + strings.TrimSpace(chapter.Name)
	}
	section := fb2.Section{Title: title}
	attachments := make(map[string]string, len(chapter.Attachments))
	for _, attachment := range chapter.Attachments {
		attachments[attachment.Name] = attachment.URL
	}
	images := make([]fb2.Image, 0)
	appendImage := func(node apiNode) error {
		for _, ref := range node.Attrs.Images {
			attachmentURL, ok := attachments[ref.Image]
			if !ok {
				return fmt.Errorf("attachment %q not found", ref.Image)
			}
			imageURL, err := resolveURL(c.imageBaseURL, attachmentURL)
			if err != nil {
				return err
			}
			image, err := c.downloadImage(ctx, imageURL, imageOffset+len(images)+1)
			if err != nil {
				return err
			}
			images = append(images, image)
			section.Elements = append(section.Elements, fb2.Element{ImageRef: image.ID})
		}
		return nil
	}
	appendBlock := func(node apiNode) error {
		var text strings.Builder
		flushText := func() {
			if value := strings.TrimSpace(text.String()); value != "" {
				section.Elements = append(section.Elements, fb2.Element{Text: value})
			}
			text.Reset()
		}
		var walkInline func(apiNode) error
		walkInline = func(current apiNode) error {
			switch current.Type {
			case "text":
				text.WriteString(current.Text)
				return nil
			case "hardBreak":
				text.WriteByte('\n')
				return nil
			case "image":
				flushText()
				return appendImage(current)
			}
			for _, child := range current.Content {
				if err := walkInline(child); err != nil {
					return err
				}
			}
			return nil
		}
		if err := walkInline(node); err != nil {
			return err
		}
		flushText()
		return nil
	}

	var walk func(apiNode) error
	walk = func(node apiNode) error {
		switch node.Type {
		case "paragraph", "heading", "blockquote", "codeBlock":
			return appendBlock(node)
		case "image":
			return appendImage(node)
		}
		for _, child := range node.Content {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, node := range chapter.Content.Content {
		if err := walk(node); err != nil {
			return fb2.Section{}, nil, err
		}
	}
	if len(section.Elements) == 0 {
		return fb2.Section{}, nil, fmt.Errorf("chapter contains no supported content")
	}
	return section, images, nil
}
