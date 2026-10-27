-- 브이월드 국가중점데이터 API 배치 적재(ned-sync)의 법정동 단위 진행 기록.
-- 한 동을 끝까지 받아 upsert한 뒤에만 기록하므로, 중간에 끊기면 그 동은 다음 실행에서 다시 받는다.
CREATE TABLE IF NOT EXISTS ned_sync_progress (
  dataset   text      NOT NULL,              -- 'price'(개별공시지가) | 'features'(토지특성)
  base_year smallint  NOT NULL,
  bcode     char(10)  NOT NULL,              -- 법정동 코드(필지 PNU 앞 10자리)
  rows      integer   NOT NULL,              -- 그 동에서 받은 행 수(API totalCount 기준 수집분)
  synced_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (dataset, base_year, bcode)
);
