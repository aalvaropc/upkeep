package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const retryMillis = 3000

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ring, ok := s.opts.Hub.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown session"})
		return
	}
	after, err := resumePoint(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	hello := map[string]any{"session": id, "last": ring.Last(), "version": s.opts.Version}
	if err := writeEvent(w, "hello", 0, hello, true); err != nil || rc.Flush() != nil {
		return
	}

	ticker := time.NewTicker(s.opts.Keepalive)
	defer ticker.Stop()
	for {
		evs, missed, changed := ring.Read(after)
		if missed {
			if err := writeEvent(w, "gap", 0, map[string]uint64{"after": after}, false); err != nil {
				return
			}
		}
		for _, e := range evs {
			if err := writeEvent(w, "", e.Seq, e, false); err != nil {
				return
			}
			after = e.Seq
		}
		if (missed || len(evs) > 0) && rc.Flush() != nil {
			return
		}
		select {
		case <-changed:
		case <-ticker.C:
			if err := writeEvent(w, "ping", 0, map[string]int64{"time": time.Now().Unix()}, false); err != nil || rc.Flush() != nil {
				return
			}
		case <-r.Context().Done():
			return
		case <-s.opts.Done:
			return
		}
	}
}

func resumePoint(r *http.Request) (uint64, error) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("after")
	}
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid resume point %q", raw)
	}
	return n, nil
}

func writeEvent(w io.Writer, name string, id uint64, data any, retry bool) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var frame []byte
	if retry {
		frame = fmt.Appendf(frame, "retry: %d\n", retryMillis)
	}
	if id > 0 {
		frame = fmt.Appendf(frame, "id: %d\n", id)
	}
	if name != "" {
		frame = fmt.Appendf(frame, "event: %s\n", name)
	}
	frame = fmt.Appendf(frame, "data: %s\n\n", payload)
	_, err = w.Write(frame)
	return err
}
