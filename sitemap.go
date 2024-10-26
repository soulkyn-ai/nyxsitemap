package nyxsitemap

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/lestrrat-go/libxml2"
	"github.com/lestrrat-go/libxml2/xsd"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const (
	sitemapExt = ".xml"
	// Reduced max URLs by 1/3 for safety
	maxURLsPerSitemap = 33333
	// Sitemap XSD schema for validation
	sitemapXSD = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
           targetNamespace="http://www.sitemaps.org/schemas/sitemap/0.9"
           elementFormDefault="qualified">
  <xs:element name="urlset">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="url" maxOccurs="unbounded">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="loc" type="xs:anyURI" />
              <xs:element name="lastmod" type="xs:date" minOccurs="0" />
              <xs:element name="changefreq" minOccurs="0">
                <xs:simpleType>
                  <xs:restriction base="xs:string">
                    <xs:enumeration value="always" />
                    <xs:enumeration value="hourly" />
                    <xs:enumeration value="daily" />
                    <xs:enumeration value="weekly" />
                    <xs:enumeration value="monthly" />
                    <xs:enumeration value="yearly" />
                    <xs:enumeration value="never" />
                  </xs:restriction>
                </xs:simpleType>
              </xs:element>
              <xs:element name="priority" minOccurs="0">
                <xs:simpleType>
                  <xs:restriction base="xs:decimal">
                    <xs:minInclusive value="0.0" />
                    <xs:maxInclusive value="1.0" />
                  </xs:restriction>
                </xs:simpleType>
              </xs:element>
            </xs:sequence>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>
`
	// Sitemap Index XSD schema for validation
	sitemapIndexXSD = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
           targetNamespace="http://www.sitemaps.org/schemas/sitemap/0.9"
           elementFormDefault="qualified">
  <xs:element name="sitemapindex">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="sitemap" maxOccurs="unbounded">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="loc" type="xs:anyURI" />
              <xs:element name="lastmod" type="xs:date" minOccurs="0" />
            </xs:sequence>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>
`
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
	LastMod string   `xml:"lastmod,omitempty"`
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
	Stylesheet  string // Holds the stylesheet URL
}

// NewSitemapOptions initializes a new SitemapOptions instance.
func NewSitemapOptions(dir string, baseURL string) *SitemapOptions {
	return &SitemapOptions{
		MaxFileSize: 52428800, // 50MB
		MaxURLs:     maxURLsPerSitemap,
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
// baseSitemapURL is the base URL where the sitemap files will be accessible.
// stylesheetURL is the URL where the stylesheet can be accessed.
func (s *SitemapOptions) Write(baseSitemapURL string, stylesheetURL string) error {
	s.Stylesheet = stylesheetURL // Store the stylesheet URL

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
		// Generate sitemap file
		err := s.writeSitemapFile("sitemap.xml", s.URLs)
		if err != nil {
			return err
		}
		// Validate the generated sitemap file
		return s.validateXMLFile(path.Join(s.Dir, "sitemap.xml"), false)
	} else {
		// Generate sitemap index
		err := s.writeSitemapIndex(baseSitemapURL)
		if err != nil {
			return err
		}
		// Validate the sitemap index and all sitemap files
		return s.validateSitemapIndexAndFiles()
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

func (s *SitemapOptions) resolveSitemapURL(baseSitemapURL, sitemapName string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseSitemapURL, "/") + "/")
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(sitemapName)
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

	// Add XML header and stylesheet with correct URL
	buffer := bytes.NewBufferString(xml.Header)
	if s.Stylesheet != "" {
		buffer.WriteString(fmt.Sprintf(`<?xml-stylesheet type="text/xsl" href="%s"?>`+"\n", s.Stylesheet))
	}
	buffer.Write(data)

	filePath := path.Join(s.Dir, filename)
	return os.WriteFile(filePath, buffer.Bytes(), 0644)
}

func (s *SitemapOptions) writeSitemapIndex(baseSitemapURL string) error {
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
		urlsSlice := s.URLs[start:end]
		err := s.writeSitemapFile(sitemapName, urlsSlice)
		if err != nil {
			return err
		}
		sitemapURL, err := s.resolveSitemapURL(baseSitemapURL, sitemapName)
		if err != nil {
			return err
		}
		index.Sitemaps = append(index.Sitemaps, Sitemap{
			Loc:     sitemapURL,
			LastMod: time.Now().UTC().Format("2006-01-02"),
		})
	}

	data, err := xml.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	// Add XML header and stylesheet with correct URL
	buffer := bytes.NewBufferString(xml.Header)
	if s.Stylesheet != "" {
		buffer.WriteString(fmt.Sprintf(`<?xml-stylesheet type="text/xsl" href="%s"?>`+"\n", s.Stylesheet))
	}
	buffer.Write(data)

	filePath := path.Join(s.Dir, "sitemap_index.xml")
	return os.WriteFile(filePath, buffer.Bytes(), 0644)
}

// validateXMLFile validates the given XML file against the sitemap XSD.
// If isIndex is true, validates against the sitemap index XSD.
func (s *SitemapOptions) validateXMLFile(filePath string, isIndex bool) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read XML file for validation: %v", err)
	}

	schemaData := sitemapXSD
	if isIndex {
		schemaData = sitemapIndexXSD
	}

	// Parse the schema
	schema, err := xsd.Parse([]byte(schemaData))
	if err != nil {
		return fmt.Errorf("failed to parse schema: %v", err)
	}
	defer schema.Free()

	// Parse the XML document
	doc, err := libxml2.Parse(data)
	if err != nil {
		return fmt.Errorf("failed to parse XML: %v", err)
	}
	defer doc.Free()

	// Validate the XML against the schema
	if err := schema.Validate(doc); err != nil {
		return fmt.Errorf("XML validation against schema failed: %v", err)
	}
	return nil
}

func (s *SitemapOptions) validateSitemapIndexAndFiles() error {
	// Validate sitemap index
	indexFilePath := path.Join(s.Dir, "sitemap_index.xml")
	if err := s.validateXMLFile(indexFilePath, true); err != nil {
		return err
	}

	// Read the sitemap index to get the list of sitemaps
	indexData, err := os.ReadFile(indexFilePath)
	if err != nil {
		return fmt.Errorf("failed to read sitemap index for validation: %v", err)
	}

	var index SitemapIndex
	if err := xml.Unmarshal(indexData, &index); err != nil {
		return fmt.Errorf("XML unmarshalling failed for sitemap index: %v", err)
	}

	// Validate each sitemap file listed in the index
	for _, sitemap := range index.Sitemaps {
		// Extract the filename from the sitemap location
		sitemapURL, err := url.Parse(sitemap.Loc)
		if err != nil {
			return fmt.Errorf("invalid sitemap URL '%s': %v", sitemap.Loc, err)
		}
		sitemapFile := path.Base(sitemapURL.Path)
		sitemapFilePath := path.Join(s.Dir, sitemapFile)

		// Validate the sitemap file
		if err := s.validateXMLFile(sitemapFilePath, false); err != nil {
			return err
		}
	}
	return nil
}
