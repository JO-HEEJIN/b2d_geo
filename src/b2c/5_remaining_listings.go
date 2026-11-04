package b2c

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Listing은 지금 팔려고 내놓은 매물 1건이다.
type Listing struct {
	ID           string    `json:"id"`
	ComplexName  string    `json:"complex_name"`
	TypeName     string    `json:"type_name,omitempty"` // 평면 타입 (모듈 2와 같은 이름)
	AskingManwon int       `json:"asking_manwon"`       // 호가 (만원)
	Floor        int       `json:"floor,omitempty"`
	Lon          float64   `json:"lon,omitempty"`
	Lat          float64   `json:"lat,omitempty"`
	Source       string    `json:"source"` // 원천 이름. 출처를 항상 남긴다
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// ListingSource는 현재 나와 있는 매물 전체를 돌려준다. 이용 계약이 있는 매물 피드나
// 중개사·집주인이 직접 등록한 매물만 원천이 될 수 있다. 포털 스크레이핑은 금지다.
type ListingSource interface {
	Fetch(ctx context.Context) ([]Listing, error)
}

// ListingChange는 Refresh 한 번에 생긴 변화다.
type ListingChange struct {
	Added []Listing // 새로 나온 매물
	Gone  []Listing // 사라진 매물 (팔렸거나 내려감)
}

// listingsOnCalendar는 systemd 타이머 주기다. 매분 0초에 돈다.
const listingsOnCalendar = "*:*:00"

// goneKeep은 사라진 매물을 "방금 팔림"으로 보여주는 기간이다.
const goneKeep = 7 * 24 * time.Hour

// RemainingListings는 남은 매물을 계속 새로 고쳐 보여준다. 매물이 워낙 빠르게
// 거래되므로 systemd 타이머(Persistent=true)가 Refresh를 매분 부르고, 상태는
// JSON 파일로 실행 사이에 이어진다.
type RemainingListings struct {
	source ListingSource
	now    func() time.Time

	mu     sync.Mutex
	active map[string]Listing
	gone   map[string]Listing
}

// NewRemainingListings는 매물 원천으로 남은 매물 목록을 만든다.
func NewRemainingListings(source ListingSource) *RemainingListings {
	return &RemainingListings{
		source: source,
		now:    time.Now,
		active: map[string]Listing{},
		gone:   map[string]Listing{},
	}
}

// Refresh는 원천에서 현재 매물을 받아 직전 상태와 비교한다. 새 매물은 Added,
// 이번에 안 보이는 매물은 Gone으로 옮긴다. 원천이 실패하면 상태를 건드리지 않는다.
func (r *RemainingListings) Refresh(ctx context.Context) (ListingChange, error) {
	current, err := r.source.Fetch(ctx)
	if err != nil {
		return ListingChange{}, fmt.Errorf("fetch listings: %w", err)
	}
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	var change ListingChange
	seen := make(map[string]bool, len(current))
	for _, l := range current {
		seen[l.ID] = true
		if prev, ok := r.active[l.ID]; ok {
			l.FirstSeen = prev.FirstSeen
		} else {
			l.FirstSeen = now
			delete(r.gone, l.ID) // 다시 올라온 매물
			change.Added = append(change.Added, l)
		}
		l.LastSeen = now
		r.active[l.ID] = l
	}
	for id, l := range r.active {
		if !seen[id] {
			delete(r.active, id)
			r.gone[id] = l
			change.Gone = append(change.Gone, l)
		}
	}
	for id, l := range r.gone {
		if now.Sub(l.LastSeen) > goneKeep {
			delete(r.gone, id)
		}
	}
	sortListings(change.Added)
	sortListings(change.Gone)
	return change, nil
}

// Remaining은 지금 남아 있는 매물 전부를 돌려준다. complexName이 비지 않으면 그
// 단지만. 추려서 줄이지 않는다. 여러 집을 둘러보는 것 자체가 즐거움이다.
func (r *RemainingListings) Remaining(complexName string) []Listing {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Listing
	for _, l := range r.active {
		if complexName == "" || l.ComplexName == complexName {
			out = append(out, l)
		}
	}
	sortListings(out)
	return out
}

// AddedSince는 since 이후 새로 나온 매물이다 ("새 매물" 알림용).
func (r *RemainingListings) AddedSince(since time.Time) []Listing {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Listing
	for _, l := range r.active {
		if !l.FirstSeen.Before(since) {
			out = append(out, l)
		}
	}
	sortListings(out)
	return out
}

// RecentlyGone은 since 이후 사라진 매물이다. "방금 팔림"을 보여주면 이 동네가
// 실제로 거래되고 있다는 것을 사용자가 눈으로 느낀다.
func (r *RemainingListings) RecentlyGone(since time.Time) []Listing {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Listing
	for _, l := range r.gone {
		if !l.LastSeen.Before(since) {
			out = append(out, l)
		}
	}
	sortListings(out)
	return out
}

type listingsState struct {
	Active []Listing `json:"active"`
	Gone   []Listing `json:"gone"`
}

// LoadState는 직전 실행이 저장한 상태를 읽는다. 파일이 없으면 빈 상태로 시작한다.
func (r *RemainingListings) LoadState(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var st listingsState
	if err := json.Unmarshal(b, &st); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active, r.gone = map[string]Listing{}, map[string]Listing{}
	for _, l := range st.Active {
		r.active[l.ID] = l
	}
	for _, l := range st.Gone {
		r.gone[l.ID] = l
	}
	return nil
}

// SaveState는 상태를 임시 파일에 쓴 뒤 이름을 바꿔 저장한다. 타이머가 실행 도중
// 끊겨도 반쯤 쓴 파일이 남지 않는다.
func (r *RemainingListings) SaveState(path string) error {
	r.mu.Lock()
	st := listingsState{}
	for _, l := range r.active {
		st.Active = append(st.Active, l)
	}
	for _, l := range r.gone {
		st.Gone = append(st.Gone, l)
	}
	r.mu.Unlock()
	sortListings(st.Active)
	sortListings(st.Gone)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".listings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// SystemdUnits는 서버에 둘 systemd 서비스와 타이머 파일 내용을 돌려준다.
// cron이 아니라 타이머를 쓰는 이유: Persistent=true라서 서버가 꺼져 있던 사이
// 놓친 실행을 부팅 직후 바로 따라잡고, 실행 기록이 journalctl에 남는다.
// execStart는 Refresh 한 번을 돌리고 끝나는 명령이다.
func (r *RemainingListings) SystemdUnits(execStart string) map[string]string {
	service := strings.Join([]string{
		"[Unit]",
		"Description=b2c remaining listings refresh",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=oneshot",
		"ExecStart=" + execStart,
		"TimeoutStartSec=50",
		"",
	}, "\n")
	timer := strings.Join([]string{
		"[Unit]",
		"Description=Refresh b2c remaining listings every minute",
		"",
		"[Timer]",
		"OnCalendar=" + listingsOnCalendar,
		"AccuracySec=1s",
		"Persistent=true",
		"Unit=b2c-listings.service",
		"",
		"[Install]",
		"WantedBy=timers.target",
		"",
	}, "\n")
	return map[string]string{
		"b2c-listings.service": service,
		"b2c-listings.timer":   timer,
	}
}

// sortListings는 단지, 호가, ID 순으로 정렬한다.
func sortListings(list []Listing) {
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.ComplexName != b.ComplexName {
			return a.ComplexName < b.ComplexName
		}
		if a.AskingManwon != b.AskingManwon {
			return a.AskingManwon < b.AskingManwon
		}
		return a.ID < b.ID
	})
}
