package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/JO-HEEJIN/b2d_geo/src/b2c"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runB2CMarket(ctx context.Context) error {
	source := b2c.MarketSource{ID: os.Getenv("B2C_MARKET_SOURCE_ID"), Name: os.Getenv("B2C_MARKET_SOURCE_NAME"), Scope: os.Getenv("B2C_MARKET_SCOPE"), ComplexName: os.Getenv("B2C_MARKET_COMPLEX_NAME")}
	if err := source.Validate(); err != nil {
		return err
	}
	endpoint := os.Getenv("B2C_MARKET_URL")
	if endpoint == "" {
		return fmt.Errorf("B2C_MARKET_URL must identify a real connected supplier; no inventory fetched")
	}
	ctx, cancel := context.WithTimeout(ctx, 165*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	store := b2c.NewListingStore(pool)
	if err = store.MarketSourceReady(ctx, source); err != nil {
		return err
	}
	feed := &b2c.HTTPMarketFeed{Source: source, URL: endpoint, Token: os.Getenv("B2C_MARKET_TOKEN")}
	snapshot, err := feed.Fetch(ctx)
	if err != nil {
		var response *b2c.MarketHTTPError
		var retry time.Duration
		if errors.As(err, &response) {
			retry = response.RetryAfter
		}
		recordCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if recordErr := store.RecordMarketFailure(recordCtx, source, "fetch_failed", retry); recordErr != nil {
			return errors.Join(err, fmt.Errorf("record market supplier availability: %w", recordErr))
		}
		return err
	}
	stats, err := store.SyncMarketSnapshot(ctx, source, snapshot)
	if err != nil {
		recordCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if recordErr := store.RecordMarketFailure(recordCtx, source, "snapshot_rejected", 0); recordErr != nil {
			return errors.Join(err, recordErr)
		}
		return err
	}
	if stats.Replay {
		fmt.Printf("b2c-listings: supplier=%s scope=%s replay=true inventory_unchanged=true\n", source.ID, source.Scope)
		return nil
	}
	fmt.Printf("b2c-listings: supplier=%s scope=%s active=%d added=%d relisted=%d price_changed=%d sold=%d withdrawn=%d missing=%d\n", source.ID, source.Scope, stats.Active, stats.Added, stats.Relisted, stats.PriceChanged, stats.Sold, stats.Withdrawn, stats.Missing)
	return nil
}
