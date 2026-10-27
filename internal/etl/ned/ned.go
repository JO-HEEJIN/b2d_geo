// Package ned는 브이월드 국가중점데이터 API(개별공시지가 getIndvdLandPriceAttr, 토지특성
// getLandCharacteristics)를 법정동 단위로 받아 official_land_prices, land_features에 적재한다.
//
// 배경: 브이월드 파일 다운로드는 브라우저 로그인 세션으로만 내려와 자동화할 수 없다. 이 API는
// pnu에 법정동 10자리를 주면 그 동의 필지 전체를 페이지 단위로 준다(2026-10-28 서버에서 확인).
// 사용 범위(지시서 Phase 7 예외, 2026-10-28): 서버 배치 ETL로 스냅샷을 만드는 용도만. API 요청 경로에서는
// 호출하지 않는다. 값은 원값 그대로 적재한다(재분류·보정 없음).
package ned

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

// Dataset은 받을 자료 종류다.
type Dataset string

const (
	Price    Dataset = "price"    // 개별공시지가
	Features Dataset = "features" // 토지특성
)

const (
	baseURL  = "https://api.vworld.kr/ned/data/"
	pageRows = 1000
	maxPages = 500 // 한 동에서 이보다 많은 페이지가 나오면 응답이 이상한 것으로 본다
)

// endpoint는 dataset별 API 이름과 응답 JSON의 최상위 키다(실제 응답에서 확인한 철자 그대로).
func (d Dataset) endpoint() (api, root string, err error) {
	switch d {
	case Price:
		return "getIndvdLandPriceAttr", "indvdLandPrices", nil
	case Features:
		return "getLandCharacteristics", "landCharacteristicss", nil
	}
	return "", "", fmt.Errorf("unknown dataset %q", d)
}

// Row는 적재 한 행이다. Price면 PricePerSqm만, Features면 네 라벨만 채운다.
type Row struct {
	PNU           string
	BaseYear      int
	PricePerSqm   int64
	LandUse       string // 토지이용상황 (ladUseSittnNm)
	TerrainHeight string // 지형높이 (tpgrphHgCodeNm)
	TerrainShape  string // 지형형상 (tpgrphFrmCodeNm)
	RoadSide      string // 도로접면 (roadSideCodeNm)
	SourceVersion time.Time
}

// Client는 API 호출기다. Key는 로그·오류 메시지에 남기지 않는다.
type Client struct {
	Key    string
	Domain string // 브이월드에 등록한 서비스 URL
	HTTP   *http.Client
	Base   string        // 테스트에서 바꾼다. 비면 baseURL
	Pause  time.Duration // 호출 사이 간격(초당 호출 수 제한용)
	Calls  int           // 지금까지 보낸 호출 수
	// RetryWait는 재시도 대기 단위다(1회째 1배, 2회째 4배). 0이면 2초. 테스트에서 줄인다.
	RetryWait time.Duration
}

// APIError는 재시도하지 않는 API 오류다(키·권한·한도 등). 이 오류가 나면 실행을 멈춘다.
type APIError struct{ Msg string }

func (e *APIError) Error() string { return "vworld api: " + e.Msg }

func (c *Client) redact(s string) string {
	if c.Key == "" {
		return s
	}
	s = strings.ReplaceAll(s, c.Key, "***")
	return strings.ReplaceAll(s, url.QueryEscape(c.Key), "***")
}

// page는 응답 JSON의 페이지 묶음이다. 숫자도 문자열로 온다.
type page struct {
	Field      []map[string]string `json:"field"`
	TotalCount string              `json:"totalCount"`
	ResultCode string              `json:"resultCode"`
	ResultMsg  string              `json:"resultMsg"`
}

// FetchDong은 한 법정동(10자리)의 해당 연도 행을 모두 받는다. 페이지를 넘기며 totalCount만큼 모으고,
// 빈 페이지가 오면 멈춘다.
func (c *Client) FetchDong(ctx context.Context, d Dataset, bcode string, year int) ([]Row, int, error) {
	api, root, err := d.endpoint()
	if err != nil {
		return nil, 0, err
	}
	var out []Row
	total, got := -1, 0
	for pageNo := 1; pageNo <= maxPages; pageNo++ {
		p, err := c.getPage(ctx, api, root, bcode, year, pageNo)
		if err != nil {
			return nil, 0, err
		}
		if total < 0 {
			total, _ = strconv.Atoi(strings.TrimSpace(p.TotalCount))
		}
		got += len(p.Field)
		for _, f := range p.Field {
			r, ok := toRow(d, f)
			if ok {
				out = append(out, r)
			}
		}
		// 서버가 요청보다 작은 페이지를 줄 수도 있어, 받은 건수가 totalCount에 닿을 때까지 넘긴다.
		if len(p.Field) == 0 || got >= total {
			return out, total, nil
		}
	}
	return nil, 0, &APIError{Msg: fmt.Sprintf("%s %s: more than %d pages", api, bcode, maxPages)}
}

func (c *Client) getPage(ctx context.Context, api, root, bcode string, year, pageNo int) (*page, error) {
	q := url.Values{}
	q.Set("key", c.Key)
	q.Set("domain", c.Domain)
	q.Set("pnu", bcode)
	q.Set("stdrYear", strconv.Itoa(year))
	q.Set("format", "json")
	q.Set("numOfRows", strconv.Itoa(pageRows))
	q.Set("pageNo", strconv.Itoa(pageNo))
	base := c.Base
	if base == "" {
		base = baseURL
	}
	u := base + api + "?" + q.Encode()

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
			continue // 네트워크·서버 오류는 재시도
		}
		if status != http.StatusOK {
			return nil, &APIError{Msg: fmt.Sprintf("%s http %d: %s", api, status, c.redact(snippet(body)))}
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, &APIError{Msg: fmt.Sprintf("%s: not json: %s", api, c.redact(snippet(body)))}
		}
		raw, ok := env[root]
		if !ok {
			// 키 오류·권한 없음·한도 초과 등은 다른 모양의 JSON으로 온다. 원문 일부를 남기고 멈춘다.
			return nil, &APIError{Msg: fmt.Sprintf("%s: unexpected response: %s", api, c.redact(snippet(body)))}
		}
		var p page
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, &APIError{Msg: fmt.Sprintf("%s: bad page: %v", api, err)}
		}
		if rc := strings.TrimSpace(p.ResultCode); rc != "" && rc != "INFO-000" {
			return nil, &APIError{Msg: fmt.Sprintf("%s: resultCode=%s msg=%s", api, rc, p.ResultMsg)}
		}
		return &p, nil
	}
	return nil, fmt.Errorf("%s %s page %d: %s", api, bcode, pageNo, c.redact(lastErr.Error()))
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
		// url.Error 안에 요청 주소(키 포함)가 들어 있어 원문을 그대로 쓰지 않는다.
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, 0, fmt.Errorf("request failed: %v", ue.Err)
		}
		return nil, 0, errors.New("request failed")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	return body, res.StatusCode, err
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// toRow는 응답 한 건을 적재 행으로 바꾼다. pnu·연도·(공시지가)가 이상한 행은 버린다.
func toRow(d Dataset, f map[string]string) (Row, bool) {
	r := Row{PNU: strings.TrimSpace(f["pnu"])}
	if len(r.PNU) != 19 {
		return r, false
	}
	y, err := strconv.Atoi(strings.TrimSpace(f["stdrYear"]))
	if err != nil || y < 1990 || y > 2100 {
		return r, false
	}
	r.BaseYear = y
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(f["lastUpdtDt"])); err == nil {
		r.SourceVersion = t
	} else {
		r.SourceVersion = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	switch d {
	case Price:
		v, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(f["pblntfPclnd"]), ",", ""), 10, 64)
		if err != nil || v <= 0 {
			return r, false
		}
		r.PricePerSqm = v
	case Features:
		r.LandUse = strings.TrimSpace(f["ladUseSittnNm"])
		r.TerrainHeight = strings.TrimSpace(f["tpgrphHgCodeNm"])
		r.TerrainShape = strings.TrimSpace(f["tpgrphFrmCodeNm"])
		r.RoadSide = strings.TrimSpace(f["roadSideCodeNm"])
	}
	return r, true
}
