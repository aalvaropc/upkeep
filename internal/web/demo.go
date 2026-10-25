package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/aalvaropc/upkeep/internal/demo"
)

const (
	maxBodyBytes   = 1 << 10
	createPerMin   = 10
	runPerMin      = 6
	retryAfterFull = "30"
)

type demoAPI struct {
	manager *demo.Manager
	info    func() map[string]any
	trust   bool
	create  *limiter
	run     *limiter
}

func (s *server) routeDemo(mux *http.ServeMux) {
	d := &demoAPI{
		manager: s.opts.Demo,
		info:    s.opts.Info,
		trust:   s.opts.TrustProxy,
		create:  newLimiter(createPerMin, time.Minute),
		run:     newLimiter(runPerMin, time.Minute),
	}
	mux.HandleFunc("GET /api/info", d.getInfo)
	mux.HandleFunc("POST /api/sessions", d.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", d.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", d.deleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/run", d.startRun)
	mux.HandleFunc("GET /api/sessions/{id}/prs/{number}", d.getPull)
}

func failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, demo.ErrUnknown), errors.Is(err, demo.ErrNoPull):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, demo.ErrBusy):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, demo.ErrFull):
		w.Header().Set("Retry-After", retryAfterFull)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "something went wrong on the server"})
	}
}

func tooMany(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests; wait a minute and try again"})
}

func (d *demoAPI) getInfo(w http.ResponseWriter, _ *http.Request) {
	info := map[string]any{}
	if d.info != nil {
		info = d.info()
	}
	info["sessions"] = d.manager.Sessions()
	writeJSON(w, http.StatusOK, info)
}

func (d *demoAPI) createSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if !d.create.allow(clientIP(r, d.trust)) {
		tooMany(w)
		return
	}
	s, err := d.manager.New(r.Context())
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Location", "/api/sessions/"+s.ID)
	writeJSON(w, http.StatusCreated, s.Snapshot(r.Context()))
}

func (d *demoAPI) getSession(w http.ResponseWriter, r *http.Request) {
	s, err := d.manager.Get(r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.Snapshot(r.Context()))
}

func (d *demoAPI) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := d.manager.Discard(r.PathValue("id")); err != nil {
		failure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *demoAPI) startRun(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if !d.run.allow(clientIP(r, d.trust)) {
		tooMany(w)
		return
	}
	position, err := d.manager.Start(r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"position": position})
}

func (d *demoAPI) getPull(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the pull request number must be a positive integer"})
		return
	}
	s, err := d.manager.Get(r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	detail, err := s.Detail(r.Context(), number)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
