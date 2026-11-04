package transit

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/text/encoding/korean"
)

func cp949(t *testing.T, s string) *bytes.Reader {
	t.Helper()
	b, err := korean.EUCKR.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

const busFixture = `정류장번호,정류장명,위도,경도,정보수집일,모바일단축번호,도시코드,도시명,관리도시명
GGB233001234,동탄역,37.200275,127.096535,2026-10-20,1234,31240,경기도 화성시,화성BIS
BSB164670201,"서면,광무교",35.15014631,129.058308,2026-10-20,5230,21,부산광역시,부산BIS
XXX1,좌표없음,,127.1,2026-10-20,,1,어딘가,x
XXX2,바다,10.0,127.1,2026-10-20,,1,어딘가,x
`

func TestParseBusCSV(t *testing.T) {
	var got []Stop
	skipped, err := ParseBusCSV(cp949(t, busFixture), func(s Stop) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 2 || len(got) != 2 {
		t.Fatalf("skipped=%d got=%+v", skipped, got)
	}
	if got[0].ID != "GGB233001234" || got[0].Name != "동탄역" || got[0].City != "경기도 화성시" || got[0].Lat != 37.200275 {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Name != "서면,광무교" {
		t.Errorf("quoted name = %q", got[1].Name)
	}
}

func TestParseBusCSVRejectsUnknownHeader(t *testing.T) {
	bad := strings.Replace(busFixture, "정류장명", "이름", 1)
	if _, err := ParseBusCSV(cp949(t, bad), func(Stop) error { return nil }); err == nil {
		t.Fatal("expected header error")
	}
}
