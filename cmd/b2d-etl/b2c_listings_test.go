package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestB2CListingHTTPEnvironment(t *testing.T) {
	t.Setenv("B2C_HTTP_TIMEOUT", "")
	t.Setenv("B2C_HTTP_RETRIES", "")
	t.Setenv("B2C_MOBILE_FALLBACK", "")
	source, err := b2cListingSourceFromEnv()
	if err != nil || source.HTTPTimeout != 40*time.Second || source.MaxRetries != 3 || !source.EnableMobileFallback {
		t.Fatalf("defaults = %+v, %v", source, err)
	}
	t.Setenv("B2C_HTTP_TIMEOUT", "12s")
	t.Setenv("B2C_HTTP_RETRIES", "0")
	t.Setenv("B2C_MOBILE_FALLBACK", "false")
	source, err = b2cListingSourceFromEnv()
	if err != nil || source.HTTPTimeout != 12*time.Second || source.MaxRetries != 0 || source.EnableMobileFallback {
		t.Fatalf("configured = %+v, %v", source, err)
	}
}

func TestB2CListingStateSourcesCannotBeMixed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listings.json")
	for _, tc := range []struct {
		stored, target string
		wantError      bool
	}{
		{"direct_registration", "direct", false}, {"sample", "direct", false},
		{"direct_registration", "naver", true}, {"naver", "direct", true}, {"naver", "naver", false},
	} {
		data := `{"active":[{"id":"1","source":"` + tc.stored + `"}],"gone":[]}`
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		err := validateB2CListingStateSource(path, tc.target)
		if (err != nil) != tc.wantError {
			t.Errorf("stored=%s target=%s error=%v", tc.stored, tc.target, err)
		}
	}
}
