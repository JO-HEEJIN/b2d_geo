package api

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamingFlushThroughRequestID(t *testing.T) {
	flushed := make(chan error, 1)
	server := httptest.NewServer(requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: ready\ndata: {}\n\n"))
		flushed <- http.NewResponseController(w).Flush()
		<-r.Context().Done()
	})))
	defer server.Close()
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Request-Id") == "" {
		t.Fatal("request ID missing")
	}
	if err := <-flushed; err != nil {
		t.Fatalf("request ID wrapper prevented flush: %v", err)
	}
	scanner := bufio.NewScanner(response.Body)
	if !scanner.Scan() || scanner.Text() != "event: ready" {
		t.Fatal("event was buffered until handler exit")
	}
}
