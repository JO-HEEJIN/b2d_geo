// Package b2c는 집을 사려는 일반 사용자(B2C)용 기능 모듈을 모은다.
//
// 모듈은 Keywords의 키 번호로 색인된다. 파일 이름 앞의 숫자가 그 키다
// (1_district.go = Keywords[1]). 모듈 하나는 파일 하나이고, 기능은 그 파일의
// 구조체 메서드로 관리한다.
//
// 판단 원칙: 집 구경은 명품 매장 아이쇼핑처럼 많이 볼수록 즐겁다. 평생 살 집이므로
// 시스템이 대신 추려서 줄이지 않는다. 정렬과 표시는 하되 후보를 지우지 않는다.
package b2c

// Keywords는 B2C 모듈 번호 -> 영문 이름이다. 원래 한국어 키워드는 주석에 남긴다.
var Keywords = map[int]string{
	1: "district",           // 동이나 구
	2: "floor_plan",         // 아파트의 구조
	3: "recent_trades",      // 최근 실거래
	4: "infra_map",          // 인프라맵
	5: "remaining_listings", // 남은 매물
}
