package b2c

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type NaverSyncStats struct{ Added, Gone, PriceChanged int }

type NaverSyncStatus struct {
	ComplexNo   string    `json:"complex_no"`
	ComplexName string    `json:"complex_name"`
	LastSuccess time.Time `json:"last_success_at"`
	ActiveCount int       `json:"active_count"`
	Fresh       bool      `json:"fresh"`
}

type NaverListingChange struct {
	ArticleNo      string    `json:"article_no"`
	Kind           string    `json:"kind"`
	PreviousManwon *int      `json:"previous_manwon"`
	CurrentManwon  *int      `json:"current_manwon"`
	DetectedAt     time.Time `json:"detected_at"`
}

// SyncNaverSnapshot commits a complete validated snapshot atomically. Direct
// registrations are never replaced, and a missing portal listing is withdrawn,
// not recorded as a confirmed sale.
func (s *ListingStore) SyncNaverSnapshot(ctx context.Context, complexNo string, listings []Listing) (NaverSyncStats, error) {
	var stats NaverSyncStats
	if complexNo == "" || strings.Contains(complexNo, ":") {
		return stats, fmt.Errorf("invalid Naver complex number")
	}
	seen := make(map[string]bool, len(listings))
	for _, l := range listings {
		if strings.TrimSpace(l.ID) == "" || l.Source != "naver" || l.ComplexName == "" || l.AskingManwon <= 0 || seen[l.ID] {
			return stats, fmt.Errorf("invalid Naver listing; database snapshot preserved")
		}
		seen[l.ID] = true
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "b2c:naver:"+complexNo); err != nil {
		return stats, err
	}
	type oldListing struct {
		name, status string
		price        int
	}
	previous := map[string]oldListing{}
	rows, err := tx.Query(ctx, `SELECT external_key,complex_name,status,asking_manwon FROM b2c_listings WHERE external_source='naver' AND external_key LIKE $1`, complexNo+":%")
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var key string
		var old oldListing
		if err := rows.Scan(&key, &old.name, &old.status, &old.price); err != nil {
			rows.Close()
			return stats, err
		}
		previous[strings.TrimPrefix(key, complexNo+":")] = old
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return stats, err
	}
	now := time.Now()
	name := ""
	event := func(id, complexName, kind string, before, after any) error {
		_, err := tx.Exec(ctx, `INSERT INTO b2c_naver_changes(complex_no,complex_name,article_no,kind,previous_manwon,current_manwon,detected_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, complexNo, complexName, id, kind, before, after, now)
		return err
	}
	for _, l := range listings {
		if name == "" {
			name = l.ComplexName
		}
		_, err := tx.Exec(ctx, `
		 INSERT INTO b2c_listings(complex_name,type_name,asking_manwon,floor,lon,lat,registered_by,external_source,external_key,status,created_at,updated_at)
		 VALUES($1,NULLIF($2,''),$3,NULLIF($4,0),NULLIF($5,0),NULLIF($6,0),'naver_sync','naver',$7,'active',$8,$8)
		 ON CONFLICT(external_source,external_key) WHERE external_source IS NOT NULL DO UPDATE SET
		 complex_name=EXCLUDED.complex_name,type_name=EXCLUDED.type_name,asking_manwon=EXCLUDED.asking_manwon,
		 floor=EXCLUDED.floor,lon=EXCLUDED.lon,lat=EXCLUDED.lat,status='active',updated_at=EXCLUDED.updated_at,
		 created_at=CASE WHEN b2c_listings.status<>'active' THEN EXCLUDED.created_at ELSE b2c_listings.created_at END`,
			l.ComplexName, l.TypeName, l.AskingManwon, l.Floor, l.Lon, l.Lat, complexNo+":"+l.ID, now)
		if err != nil {
			return stats, err
		}
		old, exists := previous[l.ID]
		if !exists || old.status != "active" {
			if err := event(l.ID, l.ComplexName, "added", nil, l.AskingManwon); err != nil {
				return stats, err
			}
			stats.Added++
		} else if old.price != l.AskingManwon {
			if err := event(l.ID, l.ComplexName, "price_changed", old.price, l.AskingManwon); err != nil {
				return stats, err
			}
			stats.PriceChanged++
		}
	}
	for id, old := range previous {
		if name == "" {
			name = old.name
		}
		if !seen[id] && old.status == "active" {
			if _, err := tx.Exec(ctx, `UPDATE b2c_listings SET status='withdrawn',updated_at=$2 WHERE external_source='naver' AND external_key=$1`, complexNo+":"+id, now); err != nil {
				return stats, err
			}
			if err := event(id, old.name, "gone", old.price, nil); err != nil {
				return stats, err
			}
			stats.Gone++
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO b2c_naver_sync(complex_no,complex_name,last_success_at,active_count) VALUES($1,$2,$3,$4) ON CONFLICT(complex_no) DO UPDATE SET complex_name=EXCLUDED.complex_name,last_success_at=EXCLUDED.last_success_at,active_count=EXCLUDED.active_count`, complexNo, name, now, len(listings))
	if err != nil {
		return stats, err
	}
	if err := tx.Commit(ctx); err != nil {
		return NaverSyncStats{}, err
	}
	return stats, nil
}

func (s *ListingStore) NaverSyncStatuses(ctx context.Context, complexName string) ([]NaverSyncStatus, error) {
	rows, err := s.pool.Query(ctx, `SELECT complex_no,complex_name,last_success_at,active_count FROM b2c_naver_sync WHERE $1='' OR complex_name=$1 ORDER BY complex_no`, complexName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NaverSyncStatus{}
	for rows.Next() {
		var status NaverSyncStatus
		if err := rows.Scan(&status.ComplexNo, &status.ComplexName, &status.LastSuccess, &status.ActiveCount); err != nil {
			return nil, err
		}
		status.Fresh = time.Since(status.LastSuccess) < 2*time.Minute
		out = append(out, status)
	}
	return out, rows.Err()
}

func (s *ListingStore) NaverRecentChanges(ctx context.Context, complexName string) ([]NaverListingChange, error) {
	rows, err := s.pool.Query(ctx, `SELECT article_no,kind,previous_manwon,current_manwon,detected_at FROM b2c_naver_changes WHERE ($1='' OR complex_name=$1) AND detected_at >= now()-interval '1 day' ORDER BY detected_at DESC,id DESC LIMIT 100`, complexName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NaverListingChange{}
	for rows.Next() {
		var change NaverListingChange
		if err := rows.Scan(&change.ArticleNo, &change.Kind, &change.PreviousManwon, &change.CurrentManwon, &change.DetectedAt); err != nil {
			return nil, err
		}
		out = append(out, change)
	}
	return out, rows.Err()
}
