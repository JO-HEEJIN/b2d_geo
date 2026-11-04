package b2c

import "testing"

func TestNaverPricesUseManwonUnits(t *testing.T) {
	for text, want := range map[string]int{
		"8억": 80000, "8억 500": 80500, "8억 5,000": 85000,
		"12.5억": 125000, "12.3456억": 123456, "85,000만원": 85000,
		"8억 500만원": 80500, "-8억": 0, "unknown": 0,
	} {
		if got := parseManwon(text); got != want {
			t.Errorf("parseManwon(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestNaverTradeLabelsAreNotFloorPlanTypes(t *testing.T) {
	source := &NaverListingSource{}
	listings := source.parseArticles(map[string]any{"articleList": []any{
		map[string]any{"articleNo": "1", "articleName": "단지", "tradeTypeName": "매매"},
		map[string]any{"articleNo": "2", "articleName": "단지", "tradeTypeName": "전세", "areaName": "127A"},
	}})
	if len(listings) != 2 || listings[0].TypeName != "" || listings[1].TypeName != "127A" {
		t.Fatalf("floor plan mapping = %+v", listings)
	}
}
