// Package api serves the plugin's HTTP routes under /plugins/{plugin_id}.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/logger"

	"github.com/schnaq/mattermost-plugin-meet/server/huddle"
)

// Service is the huddle surface the routes use; *huddle.Service implements it.
type Service interface {
	Active(userID string) ([]huddle.HuddleView, error)
	Start(ctx context.Context, userID, channelID string) (huddle.StartResult, error)
	Join(ctx context.Context, userID, huddleID string) (string, error)
	Invite(ctx context.Context, userID, huddleID string, targetIDs []string) error
	End(ctx context.Context, userID, huddleID string) error
	RespondRing(ctx context.Context, userID, ringID, action, reply string) (string, error)
}

// Handler routes the requests the server forwards to the plugin's ServeHTTP hook.
type Handler struct {
	pluginAPI plugin.API
	svc       Service
	router    *mux.Router
}

func NewHandler(pluginAPI plugin.API, svc Service) *Handler {
	h := &Handler{pluginAPI: pluginAPI, svc: svc}
	h.router = h.initRouter()
	return h
}

func (h *Handler) initRouter() *mux.Router {
	router := mux.NewRouter()

	// The Mattermost server forwards plugin requests without the panic recovery,
	// body size limit, or security headers it applies to its own REST API.
	// logRequests comes first so it records the 500 written by recoverPanic.
	router.Use(h.logRequests, h.recoverPanic, limitRequestBody, setSecurityHeaders)

	// Routes go on this subrouter. The server only checks CSRF tokens on non-GET
	// requests, so anything that changes state must use POST, PUT, PATCH or DELETE.
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(h.MattermostAuthorizationRequired)
	apiRouter.HandleFunc("/huddles", h.listHuddles).Methods(http.MethodGet)
	apiRouter.HandleFunc("/huddles", h.startHuddle).Methods(http.MethodPost)
	apiRouter.HandleFunc("/huddles/{id}/join", h.joinHuddle).Methods(http.MethodPost)
	apiRouter.HandleFunc("/huddles/{id}/invite", h.invite).Methods(http.MethodPost)
	apiRouter.HandleFunc("/huddles/{id}/end", h.endHuddle).Methods(http.MethodPost)
	apiRouter.HandleFunc("/rings/{id}", h.respondRing).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/join", h.actionJoin).Methods(http.MethodPost)

	return router
}

// ServeHTTP has the plugin hook's signature; the plugin's ServeHTTP passes its arguments straight through.
func (h *Handler) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

// MattermostAuthorizationRequired can trust Mattermost-User-ID because the server sets it only after authenticating the request.
func (h *Handler) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID(r) == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// maxRequestBodyBytes bounds how much of a request body a handler will read.
// Raise it for endpoints that legitimately accept larger payloads.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

type loggerContextKey struct{}

// requestLogger returns the logger attached by logRequests, which tags every line with the requesting user.
func requestLogger(r *http.Request) logger.Logger {
	return r.Context().Value(loggerContextKey{}).(logger.Logger)
}

// logRequests attaches a request-scoped logger and logs every request at debug level,
// since the server doesn't log plugin requests.
func (h *Handler) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log := logger.New(h.pluginAPI).With(logger.LogContext{
			"user_id": userID(r),
		})
		r = r.WithContext(context.WithValue(r.Context(), loggerContextKey{}, log))
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		// The query string is omitted because it may carry tokens or other secrets.
		log.With(logger.LogContext{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rec.status,
			"duration": time.Since(start).String(),
		}).Debugf("HTTP request")
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader records only the first status, since later ones never reach the client.
func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// recoverPanic stops a panicking handler from crashing the plugin process.
// The server deactivates a plugin that crashes repeatedly.
func (h *Handler) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if panicVal := recover(); panicVal != nil {
				requestLogger(r).With(logger.LogContext{
					"path":  r.URL.Path,
					"panic": fmt.Sprint(panicVal),
					"stack": string(debug.Stack()),
				}).Errorf("Recovered from panic in HTTP handler")
				// Once the response has started, an error body would be appended to it.
				if !rec.wroteHeader {
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}
		}()

		next.ServeHTTP(rec, r)
	})
}

func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func userID(r *http.Request) string { return r.Header.Get("Mattermost-User-Id") }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// statusOf maps a service error to its HTTP status; unknown errors are 500.
func statusOf(err error) int {
	switch {
	case errors.Is(err, huddle.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, huddle.ErrEnded):
		return http.StatusGone
	case errors.Is(err, huddle.ErrNoOwner):
		return http.StatusUnprocessableEntity
	case errors.Is(err, huddle.ErrRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, huddle.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, huddle.ErrBadRequest):
		return http.StatusBadRequest
	case errors.Is(err, huddle.ErrMeetUnavailable):
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// errorText localizes err for the requesting user; unexpected errors are logged, never shown.
func (h *Handler) errorText(r *http.Request, err error) string {
	msg, unexpected := huddle.ErrorText(huddle.Locale(h.pluginAPI.GetUser, userID(r)), err)
	if unexpected {
		requestLogger(r).Errorf("request failed: %v", err)
	}
	return msg
}

// fail writes the mapped status and a localized {"error"} body.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	writeJSON(w, statusOf(err), map[string]string{"error": h.errorText(r, err)})
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return huddle.ErrBadRequest
	}
	return nil
}

func (h *Handler) listHuddles(w http.ResponseWriter, r *http.Request) {
	views, err := h.svc.Active(userID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) startHuddle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID string `json:"channel_id"`
	}
	if err := decode(r, &req); err != nil || req.ChannelID == "" {
		h.fail(w, r, huddle.ErrBadRequest)
		return
	}
	res, err := h.svc.Start(r.Context(), userID(r), req.ChannelID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"huddle": res.Huddle, "meet_url": res.MeetURL, "already_running": res.AlreadyRunning})
}

func (h *Handler) joinHuddle(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.Join(r.Context(), userID(r), mux.Vars(r)["id"])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"meet_url": u})
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := decode(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Invite(r.Context(), userID(r), mux.Vars(r)["id"], req.UserIDs); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) endHuddle(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.End(r.Context(), userID(r), mux.Vars(r)["id"]); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respondRing: accept additionally joins the ring's huddle and returns the Meet URL.
func (h *Handler) respondRing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Text   string `json:"text"`
	}
	if err := decode(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	huddleID, err := h.svc.RespondRing(r.Context(), userID(r), mux.Vars(r)["id"], req.Action, req.Text)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if req.Action != "accept" {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	u, err := h.svc.Join(r.Context(), userID(r), huddleID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"meet_url": u})
}

// actionJoin serves the Join button of cards and invites. The server routes /plugins/... integration
// actions internally and sets Mattermost-User-Id; the body's user must match it.
func (h *Handler) actionJoin(w http.ResponseWriter, r *http.Request) {
	var req model.PostActionIntegrationRequest
	if err := decode(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.UserId != userID(r) {
		h.fail(w, r, huddle.ErrForbidden)
		return
	}
	id, _ := req.Context["huddle_id"].(string)
	u, err := h.svc.Join(r.Context(), req.UserId, id)
	if err != nil {
		// HTTP 200 so Mattermost shows the text instead of a generic action error.
		writeJSON(w, http.StatusOK, model.PostActionIntegrationResponse{EphemeralText: h.errorText(r, err)})
		return
	}
	writeJSON(w, http.StatusOK, model.PostActionIntegrationResponse{GotoLocation: u})
}
