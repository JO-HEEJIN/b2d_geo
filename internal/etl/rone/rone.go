// Package rone은 한국부동산원 R-ONE Open API에서 "(월) 용도지역별 지가변동률"(A_2024_00007)을 받아
// land_price_index에 적재한다. 손으로 받던 포털 JSON(10개월)을 대신해 2005년부터 백필하고, 이후 새 달만 받는다.
//
// 응답 구조(2026-10-28 운영 서버에서 확인): GRP_FULLNM = 지역 경로("부산>중구", "경기>화성시>만세구", "세종",
// "전남광주>광주>광산구"), CLS_FULLNM = 용도지역("주거지역" 등 10종), DTA_VAL = 변동률(%).
// 원칙: 원값 그대로 적재(재분류·보정 없음). 집계 행(전국·시도·시지역·군지역)은 건너뛴다.
package rone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// StatblID는 "(월) 용도지역별 지가변동률" 통계표다.
	StatblID = "A_2024_00007"
	baseURL  = "https://www.reb.or.kr/r-one/openapi/SttsApiTblData.do"
	pageSize = 1000
	maxPages = 50
)

// Row는 API 한 행이다.
type Row struct {
	Region string  // GRP_FULLNM, 없으면 ""(전국 집계)
	Zone   string  // CLS_FULLNM
	Month  string  // YYYYMM
	Rate   float64 // DTA_VAL
}

// Client는 API 호출기다. Key는 오류 메시지에 남기지 않는다.
type Client struct {
	Key       string
	HTTP      *http.Client
	Base      string        // 테스트에서 바꾼다
	Pause     time.Duration // 호출 사이 간격
	RetryWait time.Duration // 0이면 2초
	Calls     int
}

// APIError는 재시도하지 않는 오류(키·권한 등)다.
type APIError struct{ Msg string }

func (e *APIError) Error() string { return "r-one api: " + e.Msg }

type apiResp struct {
	Data []struct {
		Head []json.RawMessage `json:"head"`
		Row  []struct {
			Grp   *string  `json:"GRP_FULLNM"`
			Cls   *string  `json:"CLS_FULLNM"`
			Month string   `json:"WRTTIME_IDTFR_ID"`
			Val   *float64 `json:"DTA_VAL"`
		} `json:"row"`
	} `json:"SttsApiTblData"`
	Result *struct {
		Code    string `json:"CODE"`
		Message string `json:"MESSAGE"`
	} `json:"RESULT"`
}

// isNoData는 아직 공표되지 않은 달 등 "해당하는 데이터가 없습니다" 응답인지 본다.
// 운영 서버 프로브에서 문구는 확인했고 코드(INFO-200)는 공공 OpenAPI 관례라 둘 다 받는다.
func isNoData(code, msg string) bool {
	return code == "INFO-200" || strings.Contains(msg, "해당하는 데이터가 없습니다")
}

// FetchMonth는 한 달치를 모두 받는다. 아직 공표 전이면 (nil, false, nil).
func (c *Client) FetchMonth(ctx context.Context, month string) ([]Row, bool, error) {
	var out []Row
	total, got := -1, 0
	for p := 1; p <= maxPages; p++ {
		r, err := c.getPage(ctx, month, p)
		if err != nil {
			return nil, false, err
		}
		code, msg, t := pageMeta(r)
		if isNoData(code, msg) {
			if p == 1 {
				return nil, false, nil
			}
			return out, true, nil // 마지막 페이지를 넘겼다
		}
		if code != "INFO-000" {
			// 정상 응답은 항상 INFO-000을 싣는다. 코드가 없거나 다르면(키 오류 등) 멈춘다.
			return nil, false, &APIError{Msg: fmt.Sprintf("%q %s: %s", code, month, c.redact(msg))}
		}
		if total < 0 {
			total = t
		}
		n := 0
		for _, d := range r.Data {
			for _, row := range d.Row {
				n++
				if row.Val == nil || row.Cls == nil {
					continue
				}
				reg := ""
				if row.Grp != nil {
					reg = *row.Grp
				}
				out = append(out, Row{Region: reg, Zone: strings.TrimSpace(*row.Cls), Month: row.Month, Rate: *row.Val})
			}
		}
		got += n // 값이 빈 행도 센다. 건수는 원 행 기준이다
		if n == 0 || got >= total {
			return out, true, nil
		}
	}
	return nil, false, &APIError{Msg: fmt.Sprintf("%s: more than %d pages", month, maxPages)}
}

// pageMeta는 결과 코드와 전체 건수를 꺼낸다. 정상 응답은 head 안에, 오류 응답은 최상위 RESULT에 코드가 온다.
func pageMeta(r *apiResp) (code, msg string, total int) {
	if r.Result != nil {
		return r.Result.Code, r.Result.Message, 0
	}
	for _, d := range r.Data {
		for _, h := range d.Head {
			var m map[string]json.RawMessage
			if json.Unmarshal(h, &m) != nil {
				continue
			}
			if v, ok := m["list_total_count"]; ok {
				_ = json.Unmarshal(v, &total)
			}
			if v, ok := m["RESULT"]; ok {
				var res struct{ CODE, MESSAGE string }
				if json.Unmarshal(v, &res) == nil {
					code, msg = res.CODE, res.MESSAGE
				}
			}
		}
	}
	return code, msg, total
}

func (c *Client) getPage(ctx context.Context, month string, page int) (*apiResp, error) {
	q := url.Values{}
	q.Set("KEY", c.Key)
	q.Set("Type", "json")
	q.Set("STATBL_ID", StatblID)
	q.Set("DTACYCLE_CD", "MM")
	q.Set("WRTTIME_IDTFR_ID", month)
	q.Set("pIndex", strconv.Itoa(page))
	q.Set("pSize", strconv.Itoa(pageSize))
	base := c.Base
	if base == "" {
		base = baseURL
	}
	u := base + "?" + q.Encode()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			w := c.RetryWait
			if w == 0 {
				w = 2 * time.Second
			}
			time.Sleep(time.Duration(attempt*attempt) * w)
		}
		if c.Pause > 0 && c.Calls > 0 {
			time.Sleep(c.Pause)
		}
		c.Calls++
		body, status, err := c.get(ctx, u)
		if err != nil || status >= 500 {
			if err == nil {
				err = fmt.Errorf("http %d", status)
			}
			lastErr = err
			continue
		}
		if status != http.StatusOK {
			return nil, &APIError{Msg: fmt.Sprintf("http %d: %s", status, c.redact(snippet(body)))}
		}
		var r apiResp
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, &APIError{Msg: "not json: " + c.redact(snippet(body))}
		}
		return &r, nil
	}
	return nil, fmt.Errorf("%s page %d: %s", month, page, c.redact(lastErr.Error()))
}

func (c *Client) get(ctx context.Context, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, 0, fmt.Errorf("request failed: %v", ue.Err) // 요청 주소(키 포함)를 남기지 않는다
		}
		return nil, 0, errors.New("request failed")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	return body, res.StatusCode, err
}

func (c *Client) redact(s string) string {
	if c.Key == "" {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, c.Key, "***"), url.QueryEscape(c.Key), "***")
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
