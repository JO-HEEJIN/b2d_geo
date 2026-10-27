package rone

import (
	"strings"

	"github.com/JO-HEEJIN/b2d_geo/internal/etl/priceindex"
)

// place는 GRP_FULLNM 하나를 해석한 결과다. Code가 있으면 그대로 쓰고, 없으면 Name을 크로스워크에서 찾는다.
type place struct {
	Name      string // "부산광역시 중구", "경기도 수원시 장안구"
	Code      string // 세종처럼 이름으로 못 찾는 곳만 직접 준다
	Legacy    bool   // "(구)동구"처럼 폐지·개편 전 이름. 같은 코드에 현재 이름 행이 있으면 그쪽이 이긴다
	Aggregate bool   // 전국·시도·시지역·군지역 같은 집계 행(적재 대상 아님)
}

// parseRegion은 API 지역 경로를 해석한다. 시도 약칭을 모르면 Name이 비고 Aggregate도 아니다(미매핑으로 센다).
//
// 확인한 모양(2026-10-28 운영 서버 응답): "부산>중구", "경기>화성시>만세구", "세종", "인천>(구)동구",
// "전남광주>광주>광산구"(광주광역시 구), "전남광주>순천시"(전라남도 시군), 집계 "서울", "경기>시지역".
func parseRegion(grp string) place {
	grp = strings.TrimSpace(grp)
	if grp == "" {
		return place{Aggregate: true} // 전국
	}
	parts := strings.Split(grp, ">")
	var p place
	for i := range parts {
		s := strings.TrimSpace(parts[i])
		if strings.HasPrefix(s, "(구)") {
			p.Legacy = true
			s = strings.TrimSpace(strings.TrimPrefix(s, "(구)"))
		}
		if s == "시지역" || s == "군지역" {
			return place{Aggregate: true}
		}
		parts[i] = s
	}
	if len(parts) == 1 {
		if parts[0] == "세종" {
			p.Code = priceindex.SejongCode // 시군구 단계가 없는 시도
			return p
		}
		return place{Aggregate: true} // 시도·권역 집계
	}

	var sido string
	rest := parts[1:]
	if parts[0] == "전남광주" {
		// R-ONE은 광주와 전남을 한 묶음으로 낸다. 주소 DB는 광주광역시·전라남도로 나뉘어 있다.
		if parts[1] == "광주" {
			if len(parts) == 2 {
				return place{Aggregate: true} // 광주 전체
			}
			sido, rest = "광주광역시", parts[2:]
		} else {
			sido = "전라남도"
		}
	} else {
		sido = priceindex.SidoFull(parts[0])
		if sido == "" {
			return place{Legacy: p.Legacy} // 모르는 시도: 미매핑
		}
	}
	p.Name = sido + " " + strings.Join(rest, " ")
	return p
}
