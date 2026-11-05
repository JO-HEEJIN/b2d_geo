package main

import (
	"context"
	"strings"
	"testing"
)

func TestMarketModeRequiresAnActualSupplier(t *testing.T) {
	t.Setenv("B2C_LISTING_SOURCE", "market")
	t.Setenv("B2C_MARKET_SOURCE_ID", "supplier")
	t.Setenv("B2C_MARKET_SOURCE_NAME", "supplier")
	t.Setenv("B2C_MARKET_SCOPE", "complex1")
	t.Setenv("B2C_MARKET_COMPLEX_NAME", "test complex")
	t.Setenv("B2C_MARKET_URL", "")
	if err := runB2CListings(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "B2C_MARKET_URL") {
		t.Fatalf("unconfigured supplier accepted: %v", err)
	}
}
