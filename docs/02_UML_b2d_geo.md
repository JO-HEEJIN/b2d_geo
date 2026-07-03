# UML 설계 문서
# b2d_geo — 공공데이터 기반 위치 서비스 엔진

- 문서 버전: 1.0
- 작성일: 2026-07-03
- 기반 문서: 01_SDP_b2d_geo.md
- 표기: Mermaid (GitHub/Obsidian에서 렌더링 가능)

---

## 1. 컴포넌트 다이어그램 (시스템 전체 구조)

```mermaid
flowchart TB
    subgraph External["외부 데이터 소스 (공공데이터)"]
        JUSO["juso.go.kr<br/>주소 DB + 위치정보 DB"]
        SBIZ["data.go.kr<br/>상가(상권)정보"]
        NSDI["국가공간정보포털<br/>연속지적도 / 행정경계"]
    end

    subgraph ETL["b2d-etl (CLI)"]
        DL["downloader"]
        PJ["etl/juso 파서<br/>(EUC-KR 디코딩)"]
        PS["etl/sbiz 파서<br/>(CSV UTF-8)"]
        PN["etl/nsdi 로더<br/>(shapefile)"]
        VAL["스키마 검증기"]
    end

    subgraph DB["PostgreSQL 16 + PostGIS 3.4"]
        T1[("buildings<br/>주소+좌표")]
        T2[("places<br/>상가업소")]
        T3[("parcels<br/>필지 폴리곤")]
        T4[("admin_areas<br/>법정동 경계")]
        T5[("api_keys / usage_log")]
    end

    subgraph Server["b2d-server (단일 바이너리)"]
        MW["미들웨어 체인<br/>auth → ratelimit → meter"]
        H1["geocode 핸들러"]
        H2["revgeo 핸들러"]
        H3["places 핸들러"]
        H4["parcel 핸들러"]
        AP["internal/address<br/>파서/정규화 (핵심 자산)"]
        LRU["in-memory LRU 캐시"]
    end

    Client["API 소비자<br/>(프롭테크, 감정평가법인, 온프레미스 고객)"]

    JUSO --> DL
    SBIZ --> DL
    NSDI --> DL
    DL --> PJ & PS & PN
    PJ & PS & PN --> VAL --> DB

    Client -->|"HTTPS + Bearer key"| MW
    MW --> H1 & H2 & H3 & H4
    H1 & H3 --> AP
    H1 & H2 & H3 & H4 --> LRU
    H1 & H2 & H3 & H4 <-->|pgx/v5| DB
```

핵심 판단: internal/address는 geocode와 places 두 핸들러가 공유하는 유일한
도메인 모듈이며 b2d_geo의 실질 자산이다. ETL과 서버는 DB를 통해서만 결합한다
(직접 호출 없음). 온프레미스 배포 시 External을 제외한 전체가 고객 인프라로 이동한다.

---

## 2. 클래스 다이어그램 (핵심 패키지)

Go는 클래스가 없으므로 struct/interface 기준으로 표기한다.

```mermaid
classDiagram
    class AddressParser {
        <<internal/address>>
        +Parse(raw string) ParsedAddress, error
        -tokenize(raw string) []token
        -matchSido(t token) bool
        -matchRoad(t token) bool
    }
    class ParsedAddress {
        +Sido string
        +Sigungu string
        +EupMyeonDong string
        +RoadName string
        +BuildingNo string
        +Jibun string
        +IsRoadAddr bool
    }
    class Normalizer {
        <<internal/address>>
        +Normalize(s string) string
        +DecomposeJamo(s string) string
    }
    class Geocoder {
        <<internal/geocode>>
        -store Store
        -parser AddressParser
        -cache LRU
        +Geocode(ctx, query string) []Match, error
        +GeocodeBatch(ctx, queries []string) []BatchResult, error
        -exactMatch(pa ParsedAddress) []Match
        -normalizedMatch(pa ParsedAddress) []Match
        -trigramFallback(query string) []Match
    }
    class Match {
        +RoadAddress string
        +JibunAddress string
        +Lat float64
        +Lng float64
        +BuildingName string
        +PNU string
        +BCode string
        +Score float64
    }
    class ReverseGeocoder {
        <<internal/revgeo>>
        -store Store
        +Reverse(ctx, lat, lng float64) RevResult, error
    }
    class PlaceSearcher {
        <<internal/places>>
        -store Store
        -norm Normalizer
        +Search(ctx, q SearchQuery) []Place, error
        +ByCategory(ctx, code string, bbox BBox) []Place, error
    }
    class ParcelService {
        <<internal/parcel>>
        -store Store
        +ByPNU(ctx, pnu string) Parcel, error
        +ByAddress(ctx, query string) Parcel, error
        +At(ctx, lat, lng float64) Parcel, error
    }
    class Store {
        <<interface, internal/store>>
        +FindBuildingsExact(pa) []BuildingRow
        +FindBuildingsTrgm(q) []BuildingRow
        +NearestBuilding(lat, lng) BuildingRow
        +ContainingAdminArea(lat, lng) AdminRow
        +SearchPlaces(q) []PlaceRow
        +ParcelByPNU(pnu) ParcelRow
    }
    class APIKeyService {
        <<internal/auth>>
        +Issue(name string) plaintextKey, error
        +Verify(key string) KeyInfo, error
    }
    class Meter {
        <<internal/meter>>
        -ch chan UsageEvent
        +Record(e UsageEvent)
        -flushLoop()
    }

    AddressParser --> ParsedAddress
    Geocoder --> AddressParser
    Geocoder --> Match
    Geocoder --> Store
    ReverseGeocoder --> Store
    PlaceSearcher --> Normalizer
    PlaceSearcher --> Store
    ParcelService --> Store
```

설계 원칙: Store를 interface로 두는 이유는 단위 테스트에서 pgx 없이 파서와
매칭 로직을 검증하기 위해서다. 프로덕션 구현체는 pgxStore 하나만 존재한다.
불필요한 추상화 계층 추가 금지 (단순성 원칙).

---

## 3. 시퀀스 다이어그램

### 3.1 Geocoding 요청 (3단 폴백)

```mermaid
sequenceDiagram
    participant C as Client
    participant MW as Middleware
    participant G as Geocoder
    participant P as AddressParser
    participant L as LRU Cache
    participant S as pgxStore

    C->>MW: GET /v1/geocode?query=하남시 미사강변대로 100
    MW->>MW: API key 검증 (SHA-256 대조)
    MW->>MW: rate limit (token bucket)
    MW->>G: Geocode(ctx, query)
    G->>L: Get(normalizedQuery)
    alt 캐시 적중
        L-->>G: []Match
    else 캐시 미스
        G->>P: Parse(query)
        P-->>G: ParsedAddress
        G->>S: FindBuildingsExact(pa)
        alt 정확 일치 있음
            S-->>G: rows (score 1.0)
        else 정확 일치 없음
            G->>S: 정규화 일치 조회
            alt 정규화 일치 있음
                S-->>G: rows (score 0.9)
            else 최종 폴백
                G->>S: FindBuildingsTrgm(query)
                S-->>G: rows (score = similarity)
            end
        end
        G->>L: Set(normalizedQuery, matches)
    end
    G-->>MW: []Match
    MW->>MW: Meter.Record (비동기, 채널)
    MW-->>C: 200 JSON {matches:[...]}
```

### 3.2 ETL 월간 갱신

```mermaid
sequenceDiagram
    participant Op as 운영자 (Heejin)
    participant E as b2d-etl
    participant Src as juso.go.kr
    participant V as 스키마 검증기
    participant DB as PostGIS

    Op->>E: b2d-etl load --source=juso --month=2026-07
    E->>Src: 변동분 다운로드
    Src-->>E: zip (EUC-KR, 구분자 |)
    E->>E: 디코딩, 파싱
    E->>V: 컬럼 수/타입/좌표 범위 검증
    alt 명세 불일치
        V-->>E: 오류 (적재 중단)
        E-->>Op: 실패 보고 + 원인 (R-02 대응)
    else 검증 통과
        E->>DB: 스테이징 테이블 COPY
        E->>DB: ST_Transform(5179→4326), upsert
        E->>DB: 인덱스 재구축 (필요 시)
        E-->>Op: 적재 건수 + 샘플 5건 실측 대조 결과
    end
```

---

## 4. ERD (핵심 테이블)

```mermaid
erDiagram
    buildings {
        bigint id PK
        text road_address
        text jibun_address
        text building_name
        char_10 bcode "법정동코드"
        text pnu "필지 연계키"
        geometry_point geom "EPSG:4326"
        text norm_road "정규화 도로명주소 (인덱스)"
        text norm_jibun "정규화 지번주소 (인덱스)"
        text source "provenance: juso"
        date source_version
    }
    places {
        bigint id PK
        text store_number "상가업소번호"
        text name "상호명"
        text norm_name "자모분해 정규화 (trgm 인덱스)"
        char_6 category_code "업종 소분류"
        text road_address
        text jibun_address
        geometry_point geom
        text source "provenance: sbiz"
        date source_version
    }
    parcels {
        text pnu PK
        char_10 bcode
        text jibun
        geometry_polygon geom
        text land_category "지목"
        text source "provenance: nsdi"
    }
    admin_areas {
        char_10 bcode PK
        text name
        geometry_multipolygon geom
    }
    api_keys {
        bigint id PK
        text key_hash "SHA-256, 평문 저장 금지"
        text key_prefix "노출용 앞 8자"
        text owner_name
        int rate_limit_rps
        timestamptz created_at
        timestamptz revoked_at
    }
    usage_log {
        bigint id PK
        bigint api_key_id FK
        text endpoint
        int count "배치 insert 집계"
        timestamptz bucket "시간 단위 버킷"
    }

    buildings }o--|| admin_areas : "bcode"
    places }o--|| admin_areas : "bcode"
    parcels }o--|| admin_areas : "bcode"
    buildings }o--o| parcels : "pnu"
    usage_log }o--|| api_keys : "api_key_id"
```

인덱스 계획: buildings.geom / places.geom / parcels.geom / admin_areas.geom에 GiST,
buildings.norm_road / norm_jibun / places.norm_name에 gin_trgm_ops,
usage_log(api_key_id, bucket)에 복합 B-tree.

---

## 5. 배포 다이어그램

```mermaid
flowchart LR
    subgraph SaaS["SaaS 배포 (자체 운영)"]
        LB["reverse proxy<br/>(caddy, TLS)"]
        SV1["b2d-server<br/>(distroless 컨테이너)"]
        PG1[("PostGIS<br/>볼륨 영속")]
        ET1["b2d-etl<br/>(cron, 월간)"]
        LB --> SV1 --> PG1
        ET1 --> PG1
    end

    subgraph OnPrem["온프레미스 패키지 (고객 인프라)"]
        SV2["b2d-server"]
        PG2[("PostGIS<br/>데이터 스냅샷 동봉")]
        SV2 --> PG2
    end

    Dev["개발자"] -->|"docker compose up"| SaaS
    Cust["기관 고객"] -->|"오프라인 설치, 외부 통신 0건"| OnPrem
```

온프레미스 패키지는 ETL 결과 DB 스냅샷을 동봉하여 고객망에서 외부 통신 없이
동작한다. 이것이 BRD의 차별화 축 2번(온프레미스)의 기술적 근거다.
