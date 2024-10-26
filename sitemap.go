package nyxsitemap

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// SitemapURL represents a single URL entry in the sitemap.
type SitemapURL struct {
	XMLName    xml.Name `xml:"url"`
	Loc        string   `xml:"loc"`
	LastMod    string   `xml:"lastmod,omitempty"`
	ChangeFreq string   `xml:"changefreq,omitempty"`
	Priority   string   `xml:"priority,omitempty"`
}

// URLSet represents a collection of SitemapURLs.
type URLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []SitemapURL `xml:"url"`
}

// Sitemap represents a sitemap file entry in the sitemap index.
type Sitemap struct {
	XMLName xml.Name `xml:"sitemap"`
	Loc     string   `xml:"loc"`
}

// SitemapIndex represents a collection of sitemaps.
type SitemapIndex struct {
	XMLName  xml.Name  `xml:"sitemapindex"`
	Xmlns    string    `xml:"xmlns,attr"`
	Sitemaps []Sitemap `xml:"sitemap"`
}

// SitemapOptions holds configuration for generating sitemaps.
type SitemapOptions struct {
	MaxFileSize int
	MaxURLs     int
	Dir         string
	BaseURL     string
	URLs        []SitemapURL
}

// NewSitemapOptions initializes a new SitemapOptions instance.
func NewSitemapOptions(dir string, baseURL string) *SitemapOptions {
	return &SitemapOptions{
		MaxFileSize: 52428800, // 50MB
		MaxURLs:     50000,    // Maximum URLs per sitemap according to the protocol
		Dir:         dir,
		BaseURL:     strings.TrimRight(baseURL, "/"),
		URLs:        []SitemapURL{},
	}
}

// AddURL adds a single SitemapURL to the sitemap, ensuring it's valid.
func (s *SitemapOptions) AddURL(url SitemapURL) {
	if url.LastMod == "" {
		url.LastMod = time.Now().UTC().Format("2006-01-02")
	} else {
		timeLastMod, err := time.Parse("2006-01-02", url.LastMod)
		if err != nil || timeLastMod.After(time.Now().UTC()) {
			url.LastMod = time.Now().UTC().Format("2006-01-02")
		}
	}
	s.URLs = append(s.URLs, url)
}

// AddURLs adds multiple SitemapURLs to the sitemap, ensuring they're valid.
func (s *SitemapOptions) AddURLs(urls []SitemapURL) {
	for _, url := range urls {
		s.AddURL(url)
	}
}

// Write generates the sitemap files based on the current URLs.
func (s *SitemapOptions) Write() error {
	// Ensure the directory exists
	if _, err := os.Stat(s.Dir); os.IsNotExist(err) {
		if err := os.MkdirAll(s.Dir, 0755); err != nil {
			return err
		}
	}

	// Prepare URLs
	for i := range s.URLs {
		fullURL, err := s.resolveURL(s.URLs[i].Loc)
		if err != nil {
			return err
		}
		s.URLs[i].Loc = fullURL
	}

	// Decide whether to create a sitemap index or a single sitemap
	if len(s.URLs) <= s.MaxURLs {
		return s.writeSitemapFile("sitemap.xml", s.URLs)
	} else {
		return s.writeSitemapIndex()
	}
}

func (s *SitemapOptions) resolveURL(loc string) (string, error) {
	base, err := url.Parse(s.BaseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(loc)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func (s *SitemapOptions) writeSitemapFile(filename string, urls []SitemapURL) error {
	urlSet := URLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}

	data, err := xml.MarshalIndent(urlSet, "", "  ")
	if err != nil {
		return err
	}

	// Add XML header
	buffer := bytes.NewBufferString(xml.Header)
	buffer.Write(data)

	filePath := path.Join(s.Dir, filename)
	return os.WriteFile(filePath, buffer.Bytes(), 0644)
}

func (s *SitemapOptions) writeSitemapIndex() error {
	index := SitemapIndex{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
	}

	fileCount := (len(s.URLs) + s.MaxURLs - 1) / s.MaxURLs
	for i := 0; i < fileCount; i++ {
		sitemapName := fmt.Sprintf("sitemap_%d.xml", i+1)
		start := i * s.MaxURLs
		end := start + s.MaxURLs
		if end > len(s.URLs) {
			end = len(s.URLs)
		}
		err := s.writeSitemapFile(sitemapName, s.URLs[start:end])
		if err != nil {
			return err
		}
		sitemapURL, err := s.resolveURL("/" + sitemapName)
		if err != nil {
			return err
		}
		index.Sitemaps = append(index.Sitemaps, Sitemap{Loc: sitemapURL})
	}

	data, err := xml.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	// Add XML header
	buffer := bytes.NewBufferString(xml.Header)
	buffer.Write(data)

	filePath := path.Join(s.Dir, "sitemap_index.xml")
	return os.WriteFile(filePath, buffer.Bytes(), 0644)
}
