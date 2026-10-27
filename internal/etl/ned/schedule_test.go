package ned

import (
	"testing"
	"time"
)

func TestAutoYear(t *testing.T) {
	cases := map[string]int{
		"2026-05-31T23:59:00+09:00": 2025, // 5월까지는 작년
		"2026-06-01T00:00:00+09:00": 2026, // 6월부터 올해
		"2026-05-31T15:30:00Z":      2026, // UTC 5/31 15:30 = KST 6/1 00:30
		"2027-01-15T10:00:00+09:00": 2026,
	}
	for in, want := range cases {
		now, _ := time.Parse(time.RFC3339, in)
		if got := AutoYear(now); got != want {
			t.Errorf("AutoYear(%s) = %d, want %d", in, got, want)
		}
	}
}

func TestNextRun(t *testing.T) {
	cases := []struct{ now, want string }{
		{"2026-10-28T02:00:00+09:00", "2026-10-28T03:17:00+09:00"}, // 오늘 03:17 전
		{"2026-10-28T03:17:00+09:00", "2026-10-29T03:17:00+09:00"}, // 정각이면 다음 날
		{"2026-10-28T12:00:00+09:00", "2026-10-29T03:17:00+09:00"},
		{"2026-12-31T20:00:00Z", "2027-01-02T03:17:00+09:00"}, // UTC 12/31 20:00 = KST 1/1 05:00
		{"2026-10-27T18:00:00Z", "2026-10-28T03:17:00+09:00"}, // UTC 18:00 = KST 03:00
	}
	for _, c := range cases {
		now, _ := time.Parse(time.RFC3339, c.now)
		want, _ := time.Parse(time.RFC3339, c.want)
		if got := NextRun(now, 3, 17); !got.Equal(want) {
			t.Errorf("NextRun(%s) = %s, want %s", c.now, got, want)
		}
	}
}

func TestParseClock(t *testing.T) {
	if h, m, err := ParseClock("03:17"); err != nil || h != 3 || m != 17 {
		t.Fatalf("03:17 -> %d %d %v", h, m, err)
	}
	for _, bad := range []string{"", "24:00", "3", "03:60", "ab:cd"} {
		if _, _, err := ParseClock(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}
