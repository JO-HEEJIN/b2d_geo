package transit

import (
	"archive/zip"
	"bytes"
	"testing"
)

// buildXLSX는 공유 문자열 + 숫자 셀 + 빠진 셀이 섞인 최소 XLSX를 만든다.
func buildXLSX(t *testing.T, header string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	write("xl/sharedStrings.xml", `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`+
		`<si><t>역번호</t></si><si><t>`+header+`</t></si><si><t>노선번호</t></si><si><t>노선명</t></si>`+
		`<si><t>역위도</t></si><si><t>역경도</t></si><si><t>D014</t></si><si><r><t>동</t></r><r><t>천</t></r></si>`+
		`<si><t>I11D1</t></si><si><t>신분당선</t></si><si><t>101</t></si><si><t>잠실</t></si><si><t>L2</t></si></sst>`)
	write("xl/worksheets/sheet1.xml", `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`+
		`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c><c r="D1" t="s"><v>3</v></c><c r="E1" t="s"><v>4</v></c><c r="F1" t="s"><v>5</v></c></row>`+
		`<row r="2"><c r="A2" t="s"><v>6</v></c><c r="B2" t="s"><v>7</v></c><c r="C2" t="s"><v>8</v></c><c r="D2" t="s"><v>9</v></c><c r="E2"><v>37.3386841329961</v></c><c r="F2"><v>127.102793673668</v></c></row>`+
		`<row r="3"><c r="A3" t="s"><v>10</v></c><c r="B3" t="s"><v>11</v></c><c r="C3" t="s"><v>12</v></c><c r="E3"><v>37.51</v></c><c r="F3"><v>127.10</v></c></row>`+
		`<row r="4"><c r="A4" t="s"><v>10</v></c><c r="B4" t="s"><v>11</v></c><c r="C4" t="s"><v>12</v></c><c r="E4"><v>0</v></c><c r="F4"><v>0</v></c></row>`+
		`</sheetData></worksheet>`)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestParseSubwayXLSX(t *testing.T) {
	r := buildXLSX(t, "역사명")
	var got []Station
	skipped, err := ParseSubwayXLSX(r, r.Size(), func(s Station) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 || len(got) != 2 {
		t.Fatalf("skipped=%d got=%+v", skipped, got)
	}
	if got[0].ID != "D014-I11D1" || got[0].Name != "동천" || got[0].Line != "신분당선" || got[0].Lat != 37.3386841329961 {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].ID != "101-L2" || got[1].Line != "" {
		t.Errorf("second (missing D cell) = %+v", got[1])
	}
}

func TestParseSubwayXLSXRejectsUnknownHeader(t *testing.T) {
	r := buildXLSX(t, "이름")
	if _, err := ParseSubwayXLSX(r, r.Size(), func(Station) error { return nil }); err == nil {
		t.Fatal("expected header error")
	}
}

func TestColumnIndex(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "O12": 14, "Z3": 25, "AA7": 26} {
		if got := columnIndex(ref); got != want {
			t.Errorf("columnIndex(%s) = %d want %d", ref, got, want)
		}
	}
}
