# 모듈 5: 시장 매물 수집·변동 추적

현재 외부 수집·전체 스냅샷 검증·저장·변동 이력·화면 반영 경로를 구현했다.
실제 공급원은 아직 없다. `market.status=not_connected`, `listing_count=null`은 미확인이며 시장 매물이 0건이라는 뜻이 아니다.
실거래가·건축물대장·공시지가로 현재 매물을 생성하지 않는다. 네이버 수집을 전제로 하지 않는다.

## 실제 공급원 연결 조건

- 대상: 송도, 1·2동탄, 하남, 수지, 수원, 잠실의 아파트 매매. 최초 검증은 동탄2아이파크2단지.
- 개별 매물의 불변 외부 ID, 공급원 단지 ID, 타입·층·만원 단위 호가, 상태, 원천 관측 시각.
- 전체 목록과 페이지 완전성 보장. 단지별 매물 수·최저가 통계만으로는 개별 신규·소멸을 추적할 수 없다.
- 공급업체의 갱신 주기·호출 한도, 공개 웹/API 표시·저장·이력 보관 조건, 비용 확인.
- 공급원별 중복 기준. 현재 카운트는 공급원 게시매물 수이며 고유 주택 수나 시장 전수 수가 아니다.

## 수집 계약

아래는 이 프로젝트가 정의한 연동 규격이며 특정 업체가 이미 지원한다는 뜻이 아니다.
공급업체의 실제 명세를 확보하면 응답을 이 규격으로 변환하는 어댑터를 연결한다.

HTTPMarketFeed는 설정한 HTTPS endpoint를 `GET ?scope=<공급원 단지 ID>`로 호출한다.
필요한 토큰은 Authorization: Bearer 헤더에만 넣는다. URL의 인증 정보·query·외부 redirect는 허용하지 않는다.

| 응답 필드 | 의미 |
|---|---|
| snapshot_id | 관측판 ID. 동일 ID는 동일 내용 |
| scope | 설정한 공급원 단지 ID와 동일 |
| trade_type | sale 필수. 전월세 혼입 금지 |
| observed_at | 실제 목록 관측 시각, RFC3339 |
| complete | 전체 스냅샷의 완전성 보장, 반드시 true |
| total | 모든 페이지 items 총합과 정확히 동일한 정수 |
| items | null 불가. 정상 빈 목록은 명시적인 빈 배열 |
| next_cursor | 다음 페이지 cursor, 마지막은 생략/빈 문자열 |
| items[].external_id | 공급원·scope에서 불변인 매물 ID |
| items[].asking_manwon | 만원 단위 호가, 체결가 아님 |
| items[].type_name | 평면 타입. 매매/전세 라벨 아님 |
| items[].floor | 정수 층수 |
| items[].status | active / sold / withdrawn |

다음 페이지는 같은 endpoint의 cursor query로 요청한다. 모든 페이지의 ID·scope·관측 시각·total이 같아야 한다.
중복 외부 ID, 반복 cursor, 잘린 응답, 누락 필드, 알 수 없는 상태, 1만 건·100페이지 상한 초과를 거부한다.
관측 시각이 현재보다 15분 이상 오래되거나 1분 이상 미래이면 거부하며 저장된 판보다 과거인 새 판을 적용하지 않는다.
화면 신선도는 공급원 관측 시각 기준 2분이다. 1분 실행이 공급원 갱신보다 빠르다고 정보가 더 최신인 것은 아니다.

## 저장·변동 추적

- b2c_market_sources: 공급원·scope별 가용성, 시도·성공·관측 시각, 확인 건수, 다음 요청 허용 시각.
- b2c_market_snapshots: 판 ID·내용 digest. 재전송은 이력을 중복 생성하지 않고 동일 ID의 다른 내용은 거부한다.
- b2c_listings: external_source=market/<source_id>로 저장. 직접 등록·다른 공급원·다른 scope를 덮어쓰지 않는다.
- b2c_market_changes: added(신규), relisted(재등장), price_changed(호가), sold(명시적 판매 완료), withdrawn(명시적 내림), missing(완전한 후속 목록에서 빠짐).

목록에서 빠진 매물은 withdrawn으로 저장하며 판매 완료로 추정하지 않는다. 전체 반영과 이력은 단일 transaction이다.
실패·불완전 응답은 이전 목록을 보존하고 오류 상태를 남긴다. Retry-After는 DB에 저장해 후속 타이머도 대기한다.
GET /v1/b2c/listings의 market 필드는 공급 범위·가용성·게시매물 수·최근 24시간 변동(최대 100건)을 제공한다.
기존 DB 변경 SSE가 저장 후 화면 갱신을 전달한다. 직접 등록 기능도 유지한다.

## Lightsail 네이티브 실행

기존 `b2d-etl b2c-listings`에 `B2C_LISTING_SOURCE=market`을 사용하며 새 cmd를 만들지 않는다.
공급원·단지별 `/etc/b2d_geo/market/<instance>.env`를 root 소유 0600으로 두고 systemd가 읽는다.
아래 값은 실제 공급 명세·접속 정보가 확보된 뒤 설정한다. 가짜 URL·키로 실행하지 않는다.

```ini
B2C_MARKET_SOURCE_ID=<공급원 영문 ID>
B2C_MARKET_SOURCE_NAME=<표시명>
B2C_MARKET_SCOPE=<공급원 단지 ID>
B2C_MARKET_COMPLEX_NAME=<서비스 단지명과 동일>
B2C_MARKET_URL=<실제 HTTPS 피드 endpoint>
B2C_MARKET_TOKEN=<필요할 경우 실제 토큰>
```

migration 019를 먼저 적용한다. /opt/b2d_geo/current는 검증한 릴리스 디렉터리를 가리킨다.
deploy/systemd/b2c-market@.service/.timer를 설치하고 실제 instance로 1회 실행한다.
실제 수집 → 검증 → DB → API → 화면까지 확인한 instance만 매분 타이머를 활성화한다.
직접 등록 타이머와 분리하며 공급원 없이 market 타이머를 활성화하지 않는다.

## 완료 기준

테스트용 공급 서버·임시 DB 통과는 소프트웨어 검증이다. 실제 시장 연동 완료는 실제 공급원의 첫 목록을 저장하고
후속 판의 신규·호가·상태 변화를 확인한 경우다. 현재 실제 공급 연결 기준은 미충족이다.
