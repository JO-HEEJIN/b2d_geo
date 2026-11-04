# b2d_geo 현재 상태

## 운영 상태

- Phase 0~7 및 PG 심사용 Phase 4 사이트 작업 완료.
- 공개 문의 이메일은 `info@birth2death.com`이다.
- 사업자정보: 비투디, 대표 조희진, 사업자등록번호 211-25-67093,
  경기도 하남시 미사강변남로 99, 1401호, 전화 070-8983-4287.
- 통신판매업 신고번호는 `2026-경기하남-1593`이다.
- 공개 표시 서비스명은 2026-10-24부터 "올맵"이다. 도메인, API 식별자, MCP 이름(b2d-geo),
  법무 페이지 본문의 서비스명은 아직 b2d_geo다.
- 지도 화면은 4열 리포트(사진+지도 | 거래내역 | 지가변동률 | 판단 요약+출력)다.
  실거래는 `/v1/appraisal/sales/recent`가 최근 N개월을 모아 주고 6시간 캐시한다.
- 항공사진은 env `B2D_VWORLD_KEY`가 있을 때만 켜진다. Phase 7 지시서의 "V-World 실시간 API
  연동 금지"에 대해 2026-10-24 사용자가 항공사진 WMTS 타일만 예외로 허용했다(온프레미스 제외로
  NFR-08 근거 소멸). 현재 키는 개발키라 만료가 있고, 유료 판매 전에 운영키 심사를 받아 교체한다.

## Phase 4 결정 및 제약

- 공개 페이지 푸터에는 사업자정보와 이용약관·개인정보처리방침·환불정책 링크를 표시한다.
- 개인정보처리방침은 `usage_log` 이용 기록과 Caddy 접근 로그(IP 포함)를 구분한다.
- Caddy 접근 로그는 파일로 출력하고 90일 후 순환·파기한다.
- 결제 시스템 연동 전까지 Standard 신청은 이메일 접수 후 수동 발급한다.
- 포트원 경유 PG 방향은 [ADR 0001](docs/adr/0001-payment-provider-portone.md)에 기록한다.

## B2C 모듈

- 일반 사용자용 기능은 `src/b2c/` 한 패키지에 있다. 모듈 번호는 `Keywords` 사전의 키이고
  파일 이름 앞 숫자와 같다: 1 district, 2 floor_plan, 3 recent_trades, 4 infra_map,
  5 remaining_listings. 모듈 하나는 파일 하나, 기능은 구조체 메서드다. `http.go`는 API 연결만 한다.
- 공통 원칙: 후보를 추려 줄이지 않고 전부 보여준다. 판단 문구는 사람이 느끼는 감정
  (조급함, 남들도 샀다는 안도감, 계약 뒤 수리비)을 따른다.
- 구역은 송도동·1동탄·2동탄·하남·용인 수지·수원·잠실 일곱 곳뿐이다. 1/2동탄은 법정동으로 가른다
  (화성시 동탄구 41597, 2026-03-01 신설 여울동은 2동탄).
- API는 `/v1/b2c/*`다. 판단 문구가 있어 핸들러는 `internal/api`(사실 반환 경계)가 아닌 `src/b2c`에 있고,
  `server.go`는 경로만 붙인다. 읽기는 `geo` scope, 등록(`/v1/b2c/write/*`)은 `b2c_write` scope가 필요하다.
  남은 매물 목록은 `limit`(1-200, 기본 50)·`offset` 페이지로 나눠 주고 `next_offset`으로 끝까지 넘긴다.
- 실거래: 국토부 아파트 매매 API(`RTMSDataSvcAptTrade`, env `MOLIT_API_KEY`)를 월별·페이지별로 부르고 6시간 캐시한다.
- 인프라: 버스정류장은 `transit_stops`(국토부 전국 버스정류장 CSV, `b2d-etl transit-bus`)에서,
  편의점(G20405)·병원(Q10101·Q10102·Q10201)은 `places`에서 찾는다. 지하철역은 아직 적재 전이다.
  도보시간 = 직선거리 x 1.3 / 분당 80m.
- 평면 타입은 `b2c_floor_plans`, 매물은 `b2c_listings`(직접 등록만)에 있다. 매물 원천은 계약 피드와 직접 등록만
  허용한다([ADR 0002](docs/adr/0002-b2c-listing-source-no-scraping.md)).
- 남은 매물 갱신은 서버 systemd 타이머(매분, `Persistent=true`)가 compose `b2c-listings`(profile oneshot)를
  한 번씩 돌린다. 상태 파일은 호스트 `/var/lib/b2c-listings`. 설치 절차는 deploy/OPS.md 6-4.
