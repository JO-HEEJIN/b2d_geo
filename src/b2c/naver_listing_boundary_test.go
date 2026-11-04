package b2c

import (
	"testing"
	"time"
)

func TestNaverLongRetryAfterIsNotShortened(t *testing.T) {
	if got := naverRetryAfter("172800"); got != 48*time.Hour {
		t.Fatalf("Retry-After shortened: %v", got)
	}
}

func TestNaverGenericTypeNameCannotBecomeFloorPlanType(t *testing.T) {
	source := &NaverListingSource{}
	got := source.parseArticles(map[string]any{"articleList": []any{map[string]any{"articleNo": "1", "typeName": "매매", "tradeTypeName": "매매"}}})
	if len(got) != 1 || got[0].TypeName != "" {
		t.Fatalf("trade label used as floor plan: %+v", got)
	}
}
