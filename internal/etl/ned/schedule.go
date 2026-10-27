package ned

import (
	"fmt"
	"time"
)

// KST는 일정 계산 기준 시간대다. distroless 이미지엔 tzdata가 없어 고정 오프셋을 쓴다(서머타임 없음).
var KST = time.FixedZone("KST", 9*60*60)

// AutoYear는 받을 기준연도다. 개별공시지가는 매년 5월 말 공시되므로 6월부터는 올해, 그 전에는 작년.
func AutoYear(now time.Time) int {
	k := now.In(KST)
	if k.Month() < time.June {
		return k.Year() - 1
	}
	return k.Year()
}

// ParseClock는 "03:17" 같은 KST 시각을 시·분으로 바꾼다.
func ParseClock(s string) (hh, mm int, err error) {
	if _, err = fmt.Sscanf(s, "%d:%d", &hh, &mm); err != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, fmt.Errorf("time must be HH:MM (KST): %q", s)
	}
	return hh, mm, nil
}

// NextRun은 now 이후 처음 오는 KST hh:mm이다. 지금이 정확히 그 시각이면 다음 날로 넘긴다.
func NextRun(now time.Time, hh, mm int) time.Time {
	k := now.In(KST)
	t := time.Date(k.Year(), k.Month(), k.Day(), hh, mm, 0, 0, KST)
	if !t.After(k) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}
