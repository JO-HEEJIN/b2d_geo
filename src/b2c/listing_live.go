package b2c

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// listingLive shares one LISTEN connection among viewers. The listener exists
// only while a viewer is subscribed. Events invalidate views; reconnects always
// reload the DB, so a lost notification cannot leave a permanently stale list.
type listingLive struct {
	pool       *pgxpool.Pool
	mu         sync.Mutex
	clients    map[chan string]struct{}
	cancel     context.CancelFunc
	generation uint64
	connected  bool
}

func (l *listingLive) subscribe() (<-chan string, func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.clients == nil {
		l.clients = map[chan string]struct{}{}
	}
	ch := make(chan string, 1)
	l.clients[ch] = struct{}{}
	if len(l.clients) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		l.cancel = cancel
		l.generation++
		go l.listen(ctx, l.generation)
	}
	if l.connected {
		ch <- "ready"
	} else {
		ch <- "unavailable"
	}
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			delete(l.clients, ch)
			if len(l.clients) == 0 {
				l.cancel()
				l.connected = false
			}
		})
	}
}

func (l *listingLive) publish(generation uint64, event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if generation != l.generation || len(l.clients) == 0 {
		return
	}
	if event != "changed" {
		l.connected = event == "ready"
	}
	for ch := range l.clients {
		// Coalesce bursts into one reload, keeping the latest connection state.
		select {
		case <-ch:
		default:
		}
		ch <- event
	}
}

func (l *listingLive) listen(ctx context.Context, generation uint64) {
	for ctx.Err() == nil {
		connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		conn, err := pgx.ConnectConfig(connectCtx, l.pool.Config().ConnConfig.Copy())
		if err == nil {
			_, err = conn.Exec(connectCtx, "LISTEN b2c_listings_changed")
		}
		cancel()
		if err == nil {
			l.publish(generation, "ready")
			for ctx.Err() == nil {
				if _, err = conn.WaitForNotification(ctx); err != nil {
					break
				}
				l.publish(generation, "changed")
			}
		}
		if conn != nil {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
			conn.Close(closeCtx)
			closeCancel()
		}
		l.publish(generation, "unavailable")
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// listingStream carries no listing data or credentials. The normal authenticated
// read API supplies each refreshed snapshot after a committed DB notification.
func (h *Handler) listingStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	events, unsubscribe := h.live.subscribe()
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	// Bound idle HTTP sessions; the client reconnects and reloads current data.
	lifetime := time.NewTimer(55 * time.Second)
	defer lifetime.Stop()
	for {
		var message string
		select {
		case <-r.Context().Done():
			return
		case <-lifetime.C:
			return
		case event := <-events:
			message = "event: " + event + "\ndata: {}\n\n"
		case <-heartbeat.C:
			message = ": heartbeat\n\n"
		}
		if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return
		}
		if _, err := fmt.Fprint(w, message); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
	}
}
