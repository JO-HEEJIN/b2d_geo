package b2c

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/korean"
)

type scriptedSource struct {
	batches [][]Listing
	err     error
}

func (s *scriptedSource) Fetch(context.Context) ([]Listing, error) {
	if s.err != nil {
		return nil, s.err
	}
	b := s.batches[0]
	s.batches = s.batches[1:]
	return b, nil
}

func listing(id, complex string, price int) Listing {
	return Listing{ID: id, ComplexName: complex, AskingManwon: price, Source: "test"}
}

func TestRefreshTracksAddedAndGone(t *testing.T) {
	src := &scriptedSource{batches: [][]Listing{
		{listing("1", "a", 90000), listing("2", "a", 80000), listing("3", "b", 70000)},
		{listing("1", "a", 90000), listing("3", "b", 70000), listing("4", "b", 75000)},
	}}
	r := NewRemainingListings(src)
	t0 := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return t0 }
	ch, err := r.Refresh(context.Background())
	if err != nil || len(ch.Added) != 3 || len(ch.Gone) != 0 {
		t.Fatalf("first refresh: %+v %v", ch, err)
	}

	r.now = func() time.Time { return t0.Add(time.Minute) }
	ch, err = r.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Added) != 1 || ch.Added[0].ID != "4" || len(ch.Gone) != 1 || ch.Gone[0].ID != "2" {
		t.Fatalf("second refresh: %+v", ch)
	}
	rem := r.Remaining("")
	if len(rem) != 3 || rem[0].ID != "1" || rem[1].ID != "3" {
		t.Fatalf("remaining = %+v", rem)
	}
	if !rem[0].FirstSeen.Equal(t0) {
		t.Errorf("first seen moved: %v", rem[0].FirstSeen)
	}
	if got := r.Remaining("b"); len(got) != 2 {
		t.Errorf("remaining b = %d", len(got))
	}
	if got := r.AddedSince(t0.Add(time.Second)); len(got) != 1 || got[0].ID != "4" {
		t.Errorf("added since = %+v", got)
	}
	if got := r.RecentlyGone(t0); len(got) != 1 || got[0].ID != "2" {
		t.Errorf("recently gone = %+v", got)
	}
}

func TestRefreshKeepsStateOnError(t *testing.T) {
	src := &scriptedSource{batches: [][]Listing{{listing("1", "a", 1)}}}
	r := NewRemainingListings(src)
	if _, err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	src.err = errors.New("timeout")
	if _, err := r.Refresh(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if len(r.Remaining("")) != 1 {
		t.Fatal("state was cleared on fetch error")
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listings.json")
	src := &scriptedSource{batches: [][]Listing{
		{listing("1", "a", 1), listing("2", "a", 2)},
		{listing("1", "a", 1)},
	}}
	r := NewRemainingListings(src)
	for i := 0; i < 2; i++ {
		if _, err := r.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.SaveState(path); err != nil {
		t.Fatal(err)
	}
	loaded := NewRemainingListings(nil)
	if err := loaded.LoadState(path); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Remaining("")) != 1 || len(loaded.RecentlyGone(time.Time{})) != 1 {
		t.Fatalf("loaded active=%d gone=%d", len(loaded.Remaining("")), len(loaded.RecentlyGone(time.Time{})))
	}
	if err := NewRemainingListings(nil).LoadState(filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Errorf("missing state file should be empty state: %v", err)
	}
}

func TestSystemdUnits(t *testing.T) {
	units := NewRemainingListings(nil).SystemdUnits("/usr/local/bin/b2c-listings --once")
	timer := units["b2c-listings.timer"]
	for _, want := range []string{"Persistent=true", "OnCalendar=*:*:00", "Unit=b2c-listings.service", "WantedBy=timers.target"} {
		if !strings.Contains(timer, want) {
			t.Errorf("timer missing %q", want)
		}
	}
	service := units["b2c-listings.service"]
	if !strings.Contains(service, "Type=oneshot") || !strings.Contains(service, "ExecStart=/usr/local/bin/b2c-listings --once") {
		t.Errorf("service = %s", service)
	}
}

func TestPageParams(t *testing.T) {
	cases := []struct {
		limit, offset string
		l, o          int
		ok            bool
	}{
		{"", "", 0, 0, true},
		{"20", "40", 20, 40, true},
		{"0", "", 0, 0, false},
		{"201", "", 0, 0, false},
		{"10", "-1", 0, 0, false},
		{"x", "", 0, 0, false},
	}
	for _, c := range cases {
		l, o, ok := pageParams(c.limit, c.offset)
		if l != c.l || o != c.o || ok != c.ok {
			t.Errorf("pageParams(%q,%q) = %d,%d,%v", c.limit, c.offset, l, o, ok)
		}
	}
}

func TestParseListingsCSV(t *testing.T) {
	csvText := "\ufeff단지명,타입,호가(만원),층,경도,위도\n" +
		"동탄역시범,84A,\"99,000\",12,127.0961,37.2003\n" +
		"동탄역시범,,97000,,,\n" +
		",84A,90000,3,,\n" +
		"동탄역시범,84B,0,3,,\n" +
		"동탄역시범,84B,95000,고층,,\n" +
		"동탄역시범,84B,95000,3,10,10\n"
	got, bad, err := ParseListingsCSV([]byte(csvText))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].AskingManwon != 99000 || got[0].Floor != 12 || got[0].Lat != 37.2003 || got[1].TypeName != "" {
		t.Fatalf("got = %+v", got)
	}
	lines := []int{}
	for _, b := range bad {
		lines = append(lines, b.Line)
	}
	if fmt.Sprint(lines) != "[4 5 6 7]" {
		t.Errorf("bad lines = %v (%+v)", lines, bad)
	}
}

func TestParseListingsCSVEncodingAndHeader(t *testing.T) {
	cp949, err := korean.EUCKR.NewEncoder().Bytes([]byte("complex_name,asking_manwon\n하우스디더레이크,88000\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := ParseListingsCSV(cp949)
	if err != nil || len(got) != 1 || got[0].ComplexName != "하우스디더레이크" {
		t.Fatalf("cp949 got=%+v err=%v", got, err)
	}
	if _, _, err := ParseListingsCSV([]byte("name,price\nx,1\n")); err == nil {
		t.Error("expected header error")
	}
	var big strings.Builder
	big.WriteString("complex_name,asking_manwon\n")
	for i := 0; i <= listingCSVMaxRows; i++ {
		big.WriteString("a,1\n")
	}
	if _, _, err := ParseListingsCSV([]byte(big.String())); err == nil {
		t.Error("expected row limit error")
	}
}
