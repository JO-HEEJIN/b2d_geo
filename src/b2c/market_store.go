package b2c

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type MarketSyncStats struct {
	Added, Relisted, PriceChanged, Sold, Withdrawn, Missing, Active int
	Replay                                                          bool
}

// SyncMarketSnapshot applies only a validated complete snapshot. Supplier and
// complex scope are isolated; an absent item is withdrawn, never inferred sold.
func (s *ListingStore) SyncMarketSnapshot(ctx context.Context, source MarketSource, snapshot MarketSnapshot) (MarketSyncStats, error) {
	var stats MarketSyncStats
	if err := snapshot.Validate(source, time.Now()); err != nil {
		return stats, err
	}
	snapshot.ObservedAt = snapshot.ObservedAt.UTC().Truncate(time.Microsecond)
	snapshot.Items = append([]MarketListing{}, snapshot.Items...)
	sort.Slice(snapshot.Items, func(i, j int) bool { return snapshot.Items[i].ExternalID < snapshot.Items[j].ExternalID })
	raw, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "market:"+source.ID+":"+source.Scope); err != nil {
		return stats, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO b2c_market_sources(source_id,scope,name,complex_name) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, source.ID, source.Scope, source.Name, source.ComplexName); err != nil {
		return stats, err
	}
	var existingComplex string
	var observed *time.Time
	if err = tx.QueryRow(ctx, `SELECT complex_name,observed_at FROM b2c_market_sources WHERE source_id=$1 AND scope=$2`, source.ID, source.Scope).Scan(&existingComplex, &observed); err != nil {
		return stats, err
	}
	if existingComplex != source.ComplexName {
		return stats, fmt.Errorf("market source scope already belongs to another complex")
	}
	var priorDigest *string
	if err = tx.QueryRow(ctx, `SELECT (SELECT digest FROM b2c_market_snapshots WHERE source_id=$1 AND scope=$2 AND snapshot_id=$3)`, source.ID, source.Scope, snapshot.ID).Scan(&priorDigest); err != nil {
		return stats, err
	}
	if priorDigest != nil {
		if *priorDigest != digest {
			return stats, fmt.Errorf("market snapshot id reused with different content")
		}
		if _, err = tx.Exec(ctx, `UPDATE b2c_market_sources SET last_attempt_at=now(),last_success_at=now(),last_error='',next_attempt_at=NULL WHERE source_id=$1 AND scope=$2 AND snapshot_id=$3`, source.ID, source.Scope, snapshot.ID); err != nil {
			return stats, err
		}
		if err = tx.Commit(ctx); err != nil {
			return stats, err
		}
		stats.Replay = true
		return stats, nil
	}
	if observed != nil && !snapshot.ObservedAt.After(*observed) {
		return stats, fmt.Errorf("out-of-order market snapshot rejected")
	}
	type prior struct {
		id     int64
		status string
		price  int
	}
	previous := map[string]prior{}
	externalSource := "market/" + source.ID
	rows, err := tx.Query(ctx, `SELECT id,external_key,status,asking_manwon FROM b2c_listings WHERE external_source=$1 AND split_part(external_key,':',1)=$2`, externalSource, source.Scope)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var old prior
		var key string
		if err = rows.Scan(&old.id, &key, &old.status, &old.price); err != nil {
			rows.Close()
			return stats, err
		}
		previous[strings.TrimPrefix(key, source.Scope+":")] = old
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return stats, err
	}
	event := func(id int64, externalID, kind string, before, after any) error {
		_, e := tx.Exec(ctx, `INSERT INTO b2c_market_changes(source_id,scope,snapshot_id,listing_id,external_id,complex_name,kind,previous_manwon,current_manwon,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, source.ID, source.Scope, snapshot.ID, id, externalID, source.ComplexName, kind, before, after, snapshot.ObservedAt)
		return e
	}
	seen := map[string]bool{}
	for _, item := range snapshot.Items {
		seen[item.ExternalID] = true
		if item.Status == "active" {
			stats.Active++
		}
		var id int64
		err = tx.QueryRow(ctx, `INSERT INTO b2c_listings(complex_name,type_name,asking_manwon,floor,registered_by,external_source,external_key,status)
		 VALUES($1,NULLIF($2,''),$3,NULLIF($4,0),'market_sync',$5,$6,$7)
		 ON CONFLICT(external_source,external_key) WHERE external_source IS NOT NULL DO UPDATE SET
		 type_name=EXCLUDED.type_name,asking_manwon=EXCLUDED.asking_manwon,floor=EXCLUDED.floor,status=EXCLUDED.status,
		 updated_at=CASE WHEN ROW(b2c_listings.type_name,b2c_listings.asking_manwon,b2c_listings.floor,b2c_listings.status) IS DISTINCT FROM ROW(EXCLUDED.type_name,EXCLUDED.asking_manwon,EXCLUDED.floor,EXCLUDED.status) THEN now() ELSE b2c_listings.updated_at END
		 RETURNING id`, source.ComplexName, item.TypeName, item.AskingManwon, item.Floor, externalSource, source.Scope+":"+item.ExternalID, item.Status).Scan(&id)
		if err != nil {
			return stats, err
		}
		old, exists := previous[item.ExternalID]
		kind := ""
		if !exists {
			if item.Status == "active" {
				kind = "added"
				stats.Added++
			} else {
				kind = item.Status
			}
		} else if old.status != item.Status {
			kind = item.Status
			if item.Status == "active" {
				kind = "relisted"
				stats.Relisted++
			}
		}
		if kind != "" {
			var before any
			if exists {
				before = old.price
			}
			if err = event(id, item.ExternalID, kind, before, item.AskingManwon); err != nil {
				return stats, err
			}
			if kind == "sold" {
				stats.Sold++
			}
			if kind == "withdrawn" {
				stats.Withdrawn++
			}
		}
		if exists && old.price != item.AskingManwon && item.Status == "active" {
			if err = event(id, item.ExternalID, "price_changed", old.price, item.AskingManwon); err != nil {
				return stats, err
			}
			stats.PriceChanged++
		}
	}
	for externalID, old := range previous {
		if seen[externalID] || old.status != "active" {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE b2c_listings SET status='withdrawn',updated_at=now() WHERE id=$1`, old.id); err != nil {
			return stats, err
		}
		if err = event(old.id, externalID, "missing", old.price, nil); err != nil {
			return stats, err
		}
		stats.Missing++
	}
	if _, err = tx.Exec(ctx, `INSERT INTO b2c_market_snapshots(source_id,scope,snapshot_id,observed_at,digest,item_count) VALUES($1,$2,$3,$4,$5,$6)`, source.ID, source.Scope, snapshot.ID, snapshot.ObservedAt, digest, len(snapshot.Items)); err != nil {
		return stats, err
	}
	if _, err = tx.Exec(ctx, `UPDATE b2c_market_sources SET name=$3,last_attempt_at=now(),last_success_at=now(),observed_at=$4,snapshot_id=$5,last_error='',next_attempt_at=NULL,active_count=$6 WHERE source_id=$1 AND scope=$2`, source.ID, source.Scope, source.Name, snapshot.ObservedAt, snapshot.ID, stats.Active); err != nil {
		return stats, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MarketSyncStats{}, err
	}
	return stats, nil
}

// RecordMarketFailure records availability without clearing a previous snapshot.
func (s *ListingStore) RecordMarketFailure(ctx context.Context, source MarketSource, cause string, retryAfter time.Duration) error {
	if err := source.Validate(); err != nil {
		return err
	}
	if cause != "fetch_failed" && cause != "snapshot_rejected" {
		return fmt.Errorf("invalid market failure classification")
	}
	var retryAt *time.Time
	if retryAfter > 0 {
		t := time.Now().Add(retryAfter)
		retryAt = &t
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO b2c_market_sources(source_id,scope,name,complex_name,last_error,next_attempt_at) VALUES($1,$2,$3,$4,$5,$6)
	 ON CONFLICT(source_id,scope) DO UPDATE SET last_attempt_at=now(),last_error=EXCLUDED.last_error,next_attempt_at=EXCLUDED.next_attempt_at WHERE b2c_market_sources.complex_name=EXCLUDED.complex_name`, source.ID, source.Scope, source.Name, source.ComplexName, cause, retryAt)
	return err
}

func (s *ListingStore) MarketSourceReady(ctx context.Context, source MarketSource) error {
	var retryAt *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT next_attempt_at FROM b2c_market_sources WHERE source_id=$1 AND scope=$2)`, source.ID, source.Scope).Scan(&retryAt); err != nil {
		return err
	}
	if retryAt != nil && retryAt.After(time.Now()) {
		return fmt.Errorf("market supplier retry deadline is %s; no request sent", retryAt.UTC().Format(time.RFC3339))
	}
	return nil
}

type MarketSourceStatus struct {
	MarketSource
	LastSuccess *time.Time `json:"last_success_at"`
	ObservedAt  *time.Time `json:"observed_at"`
	LastError   string     `json:"last_error,omitempty"`
	ActiveCount *int       `json:"active_count"`
	Fresh       bool       `json:"fresh"`
}
type MarketChange struct {
	ID             string    `json:"id"`
	SourceID       string    `json:"source_id"`
	ExternalID     string    `json:"external_id"`
	Kind           string    `json:"kind"`
	PreviousManwon *int      `json:"previous_manwon"`
	CurrentManwon  *int      `json:"current_manwon"`
	ObservedAt     time.Time `json:"observed_at"`
	DetectedAt     time.Time `json:"detected_at"`
}
type MarketCoverage struct {
	Status       string               `json:"status"`        // not_connected, unavailable, partial, ready, stale
	ListingCount *int                 `json:"listing_count"` // null means unknown, not zero
	Sources      []MarketSourceStatus `json:"sources"`
	Changes      []MarketChange       `json:"changes"`
}

func (s *ListingStore) MarketCoverage(ctx context.Context, complexName string) (MarketCoverage, error) {
	result := MarketCoverage{Status: "not_connected", Sources: []MarketSourceStatus{}, Changes: []MarketChange{}}
	rows, err := s.pool.Query(ctx, `SELECT source_id,scope,name,complex_name,last_success_at,observed_at,last_error,active_count FROM b2c_market_sources WHERE $1='' OR complex_name=$1 ORDER BY source_id,scope`, complexName)
	if err != nil {
		return result, err
	}
	known, count, fresh := 0, 0, 0
	for rows.Next() {
		var status MarketSourceStatus
		if err = rows.Scan(&status.ID, &status.Scope, &status.Name, &status.ComplexName, &status.LastSuccess, &status.ObservedAt, &status.LastError, &status.ActiveCount); err != nil {
			rows.Close()
			return result, err
		}
		status.Fresh = status.ObservedAt != nil && time.Since(*status.ObservedAt) < 2*time.Minute && status.LastError == ""
		if status.Fresh {
			fresh++
		}
		if status.ActiveCount != nil {
			known++
			count += *status.ActiveCount
		}
		result.Sources = append(result.Sources, status)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Sources) > 0 {
		result.Status = "unavailable"
		if known == len(result.Sources) {
			result.ListingCount = &count
			result.Status = "stale"
		} else if known > 0 {
			result.Status = "partial"
		}
		if fresh == len(result.Sources) {
			result.Status = "ready"
		}
	}
	rows, err = s.pool.Query(ctx, `SELECT id,source_id,external_id,kind,previous_manwon,current_manwon,observed_at,detected_at FROM b2c_market_changes WHERE ($1='' OR complex_name=$1) AND detected_at>=now()-interval '1 day' ORDER BY detected_at DESC,id DESC LIMIT 100`, complexName)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var change MarketChange
		if err = rows.Scan(&id, &change.SourceID, &change.ExternalID, &change.Kind, &change.PreviousManwon, &change.CurrentManwon, &change.ObservedAt, &change.DetectedAt); err != nil {
			return result, err
		}
		change.ID = strconv.FormatInt(id, 10)
		result.Changes = append(result.Changes, change)
	}
	return result, rows.Err()
}
