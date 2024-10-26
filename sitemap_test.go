package nyxsitemap

import (
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
)

func TestSitemapGeneration(t *testing.T) {
	dir := "./test_sitemaps"
	baseURL := "https://www.example.com"

	// Clean up before test
	os.RemoveAll(dir)

	sm := NewSitemapOptions(dir, baseURL)

	sm.AddURL(SitemapURL{
		Loc:        "/",
		LastMod:    "2023-10-25",
		ChangeFreq: "daily",
		Priority:   "1.0",
	})

	sm.AddURL(SitemapURL{
		Loc:        "/about",
		LastMod:    "invalid-date", // Should be replaced with current date
		ChangeFreq: "monthly",
		Priority:   "0.8",
	})

	// Add more URLs to trigger sitemap index creation
	for i := 0; i < 60000; i++ {
		sm.AddURL(SitemapURL{
			Loc:        "/page/" + strconv.Itoa(i),
			ChangeFreq: "weekly",
			Priority:   "0.5",
		})
	}

	err := sm.Write()
	if err != nil {
		t.Fatalf("Error writing sitemaps: %v", err)
	}

	// Check if sitemap index was created
	indexFile := path.Join(dir, "sitemap_index.xml")
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		t.Fatalf("Sitemap index not created")
	}

	// Read and validate the sitemap index
	data, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatalf("Error reading sitemap index: %v", err)
	}

	if !strings.Contains(string(data), "<sitemapindex") {
		t.Fatalf("Invalid sitemap index content")
	}

	// Clean up after test
	os.RemoveAll(dir)
}
