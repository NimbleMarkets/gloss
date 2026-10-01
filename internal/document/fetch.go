package document

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/NimbleMarkets/ntcharts-svg/svg"
)

// Fetch downloads what a URL names into dir and returns its path. It takes
// only http and https, follows redirects only to the same, reads no more
// than a file may be, and keeps only what gloss can show. Nothing is fetched
// unless the user asked: gloss does not reach the network on its own.
func Fetch(ctx context.Context, address, dir string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("only http and https addresses are fetched")
	}
	client := &http.Client{CheckRedirect: fetchRedirect}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gloss")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: %s", svg.SanitizeForTerminal(u.Host), sanitizeErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: HTTP %d", svg.SanitizeForTerminal(u.Host), resp.StatusCode)
	}
	name := fetchedName(resp.Request.URL, resp.Header.Get("Content-Type"))
	f, err := exclusive(dir, name)
	if err != nil {
		return "", err
	}
	size, copyErr := io.Copy(f, io.LimitReader(resp.Body, MaxFileBytes+1))
	if err := errors.Join(copyErr, f.Close()); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("%s: %s", svg.SanitizeForTerminal(u.Host), sanitizeErr(err))
	}
	if size > MaxFileBytes {
		os.Remove(f.Name())
		return "", fmt.Errorf("%s exceeds 128 MiB", svg.SanitizeForTerminal(name))
	}
	if _, err := Probe(f.Name(), ""); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("%s: %s", svg.SanitizeForTerminal(name), SkipReason(err))
	}
	return f.Name(), nil
}

// fetchRedirect decides whether a redirect is followed: only on the web, only
// ten times, and never from https to http, which would send in the clear what
// was asked for over a secure connection.
func fetchRedirect(r *http.Request, via []*http.Request) error {
	if r.URL.Scheme != "http" && r.URL.Scheme != "https" {
		return errors.New("redirected away from the web")
	}
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	if r.URL.Scheme == "http" {
		for _, earlier := range via {
			if earlier.URL.Scheme == "https" {
				return errors.New("redirected from https to http")
			}
		}
	}
	return nil
}

func sanitizeErr(err error) string {
	// Network errors repeat the address, which came from a file.
	return svg.SanitizeForTerminal(err.Error())
}

// fetchedName names a download after the end of its address, with an
// extension from the content type where the address has none.
func fetchedName(u *url.URL, contentType string) string {
	name := path.Base(u.Path)
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		name = "download"
	}
	if filepath.Ext(name) == "" {
		kind, _, _ := mime.ParseMediaType(contentType)
		if ext, ok := map[string]string{
			"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp", "image/bmp": ".bmp", "image/tiff": ".tiff", "image/heic": ".heic", "image/heif": ".heif",
			"image/svg+xml": ".svg", "application/pdf": ".pdf", "model/stl": ".stl", "application/sla": ".stl", "model/3mf": ".3mf", "application/vnd.ms-package.3dmanufacturing-3dmodel+xml": ".3mf",
			"text/markdown": ".md", "text/plain": ".md", "text/csv": ".csv", "text/tab-separated-values": ".tsv",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
		}[kind]; ok {
			name += ext
		}
	}
	return name
}

// exclusive creates a file of the name, or the nearest free one.
func exclusive(dir, name string) (*os.File, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n < 1000; n++ {
		if n > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, n, ext)
		}
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, fmt.Errorf("%s: too many downloads with this name", name)
}

// URL is the web address a cell holds or links to, if any.
func (s *Sheet) URL(row, col int) string {
	if link, ok := s.links[[2]int{row, col}]; ok {
		return link
	}
	if row < 0 || row >= len(s.Rows) || col < 0 || col >= len(s.Rows[row]) {
		return ""
	}
	text := strings.TrimSpace(s.Rows[row][col])
	if (strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://")) && !strings.ContainsAny(text, " \t") {
		if u, err := url.Parse(text); err == nil && u.Host != "" {
			return text
		}
	}
	return ""
}
