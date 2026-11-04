# b2d_geo

> b2c 브랜치: 일반 사용자(B2C)용 기능은 [`src/b2c/README.md`](src/b2c/README.md)를 본다.

공공데이터 기반 위치 서비스 엔진. 카카오/네이버 API 의존 없이 geocoding,
reverse geocoding, keyword search, places를 제공하고, 부동산 특화 기능
(PNU, 지적, 필지 조회)을 갖춘 상용 API 서버.

## 스택

Go 1.23+ (net/http), PostgreSQL 16 + PostGIS 3.4, pgx/v5, pg_trgm.
프로덕션 직접 의존성 5개 이하 유지.

## 개발 환경

```sh
make up       # PostGIS 컨테이너 기동 (호스트 포트 5433)
make migrate  # 스키마 마이그레이션
make build    # bin/b2d-server, bin/b2d-etl
make test
make psql     # 컨테이너 psql 접속
```

DB 접속은 `B2D_DATABASE_URL` 환경변수로 재정의 가능
(기본: `postgres://b2d:...@localhost:5433/b2d_geo`).

## 문서

- 설계 문서 6종: `docs/` (SDP, UML, SRS, BRD, TestCases, UserProcessFlow)
- 진행 계획: `tasks/todo.md`
- 학습 기록: `learning_log.md`

## 데이터 출처

주소기반산업지원서비스(juso.go.kr), 소상공인시장진흥공단 상가(상권)정보(data.go.kr),
국가공간정보포털(nsdi.go.kr). 공공데이터만 사용하며 전 레코드에 provenance
(source, source_version)를 기록한다.
