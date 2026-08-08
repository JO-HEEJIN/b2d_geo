// 요청 ID 미들웨어: 모든 /v1 요청에 X-Request-Id 응답 헤더를 부여해 브라우저
// 콘솔 에러와 서버 로그를 상호 대조할 수 있게 한다. 프론트(shared.js)는 실패 시
// 이 ID를 콘솔과 화면에 함께 남기고, 서버는 에러 응답(>=400)만 ID·상태·소요시간과
// 함께 로그한다 (정상 트래픽 소음 방지; 사용량 집계는 meter가 담당).
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

type reqIDKey struct{}

// RequestID는 미들웨어가 심은 요청 ID를 돌려준다 (없으면 "").
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(reqIDKey{}).(string)
	return id
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b [6]byte
		rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		w.Header().Set("X-Request-Id", id)
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), reqIDKey{}, id)))
		if sw.status >= 400 {
			log.Printf("req %s: %s %s -> %d (%s)",
				id, r.Method, r.URL.RequestURI(), sw.status,
				time.Since(start).Round(time.Millisecond))
		}
	})
}

// statusRecorder는 핸들러가 쓴 상태 코드를 기록한다 (WriteHeader 미호출 시 200).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
