package rone

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 정상 응답 모양(2026-10-28 운영 서버 프로브): head에 건수와 RESULT, 다음 원소에 row.
const okPage = `{"SttsApiTblData":[{"head":[{"list_total_count":%d},{"RESULT":{"CODE":"INFO-000","MESSAGE":"정상 처리되었습니다."}}]},{"row":[%s]}]}`
const rowTmpl = `{"WRTTIME_IDTFR_ID":"%s","GRP_FULLNM":%s,"CLS_FULLNM":"%s","ITM_FULLNM":"지가변동률","DTA_VAL":%s}`
const noDataPage = `{"RESULT":{"CODE":"INFO-200","MESSAGE":"해당하는 데이터가 없습니다."}}`

func TestFetchMonthPagesUntilTotal(t *testing.T) {
	// 3건을 1건씩 3페이지로 준다. 값이 빈 행(null)도 건수에는 들어가므로 끝까지 넘겨야 한다.
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("KEY") != "SECRET" || q.Get("STATBL_ID") != StatblID || q.Get("DTACYCLE_CD") != "MM" || q.Get("WRTTIME_IDTFR_ID") != "202606" || q.Get("Type") != "json" {
			t.Errorf("bad query: %v", q)
		}
		var row string
		switch q.Get("pIndex") {
		case "1":
			row = fmt.Sprintf(rowTmpl, "202606", `"부산>중구"`, "주거지역", "0.016")
		case "2":
			row = fmt.Sprintf(rowTmpl, "202606", `"부산>중구"`, "상업지역", "null")
		case "3":
			row = fmt.Sprintf(rowTmpl, "202606", "null", "주거지역", "-0.1")
		}
		fmt.Fprintf(w, okPage, 3, row)
	}))
	defer srv.Close()
	c := &Client{Key: "SECRET", Base: srv.URL}
	rows, ok, err := c.FetchMonth(context.Background(), "202606")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if calls != 3 || len(rows) != 2 {
		t.Fatalf("calls=%d rows=%d, want 3/2", calls, len(rows))
	}
	if rows[0].Region != "부산>중구" || rows[0].Zone != "주거지역" || rows[0].Rate != 0.016 || rows[0].Month != "202606" {
		t.Fatalf("row0 %+v", rows[0])
	}
	if rows[1].Region != "" || rows[1].Rate != -0.1 {
		t.Fatalf("row1 (전국 집계) %+v", rows[1])
	}
}

func TestFetchMonthNoDataAndAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("WRTTIME_IDTFR_ID") == "209912" {
			fmt.Fprint(w, noDataPage)
			return
		}
		// 키 오류. 원문에 키가 섞여도 오류 메시지엔 남지 않아야 한다.
		fmt.Fprint(w, `{"RESULT":{"CODE":"ERROR-290","MESSAGE":"인증키 SECRET 가 유효하지 않습니다."}}`)
	}))
	defer srv.Close()
	c := &Client{Key: "SECRET", Base: srv.URL}
	rows, ok, err := c.FetchMonth(context.Background(), "209912")
	if err != nil || ok || rows != nil {
		t.Fatalf("no data: rows=%v ok=%v err=%v", rows, ok, err)
	}
	_, _, err = c.FetchMonth(context.Background(), "202606")
	var ae *APIError
	if !errors.As(err, &ae) || !strings.Contains(err.Error(), "ERROR-290") {
		t.Fatalf("want APIError ERROR-290, got %v", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("key leaked: %v", err)
	}
}

func TestFetchMonthUnknownShapeStops(t *testing.T) {
	// 결과 코드가 아예 없는 응답(HTML 오류 페이지 등을 JSON으로 착각한 경우)을 빈 달로 넘기면 안 된다.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"something":"else"}`)
	}))
	defer srv.Close()
	c := &Client{Key: "k", Base: srv.URL}
	_, ok, err := c.FetchMonth(context.Background(), "202606")
	var ae *APIError
	if ok || !errors.As(err, &ae) {
		t.Fatalf("want APIError, got ok=%v err=%v", ok, err)
	}
}

func TestServerErrorRetriesThenFailsWithoutKey(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &Client{Key: "SECRET", Base: srv.URL, RetryWait: time.Millisecond}
	_, _, err := c.FetchMonth(context.Background(), "202606")
	if err == nil || calls != 3 {
		t.Fatalf("want 3 attempts and error, got calls=%d err=%v", calls, err)
	}
	var ae *APIError
	if errors.As(err, &ae) {
		t.Fatalf("5xx should be retryable error, not APIError: %v", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("key leaked: %v", err)
	}
}
