package web

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrandAssetEmbedded(t *testing.T) {
	const asset = "brand/b2d-geo-c1abd2177e38.png"
	b, err := fs.ReadFile(Assets, asset)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != "c1abd2177e3819c5d37fb68c7865f852ce1cc739dac6eebd8b55752527485545" {
		t.Fatal("brand image differs from the supplied original")
	}
	info, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil || info.Width != 512 || info.Height != 512 {
		t.Fatalf("invalid brand PNG: %+v %v", info, err)
	}
	w := httptest.NewRecorder()
	http.FileServerFS(Assets).ServeHTTP(w, httptest.NewRequest("GET", "/"+asset, nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("asset not served correctly: %d %v", w.Code, w.Header())
	}
}

func TestEveryPageUsesBrandAsset(t *testing.T) {
	pages, err := fs.Glob(Assets, "*.html")
	if err != nil || len(pages) == 0 {
		t.Fatalf("no HTML pages: %v", err)
	}
	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			b, err := fs.ReadFile(Assets, page)
			if err != nil {
				t.Fatal(err)
			}
			html := string(b)
			for _, required := range []string{
				`<img class="brand-logo" src="/brand/b2d-geo-c1abd2177e38.png"`,
				`rel="icon" type="image/png" sizes="512x512" href="/brand/b2d-geo-c1abd2177e38.png"`,
				`rel="apple-touch-icon" href="/brand/b2d-geo-c1abd2177e38.png"`,
				`property="og:image" content="https://api.birth2death.com/brand/b2d-geo-c1abd2177e38.png"`,
				`href="/styles.css?v=brand-c1abd2177e38"`,
			} {
				if !strings.Contains(html, required) {
					t.Errorf("missing brand reference: %s", required)
				}
			}
			if strings.Contains(html, `class="glyph"`) {
				t.Fatal("old placeholder logo remains")
			}
		})
	}
}
