package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/stretchr/testify/require"

	"github.com/schnaq/mattermost-plugin-meet/server/huddle"
)

type stubAPI struct{ plugin.API }

func (stubAPI) LogDebug(string, ...any) {}
func (stubAPI) LogError(string, ...any) {}
func (stubAPI) GetUser(id string) (*model.User, *model.AppError) {
	return &model.User{Id: id, Locale: "en"}, nil
}

type fakeSvc struct {
	err       error
	huddleID  string // returned by RespondRing
	joined    []string
	joinedErr error
}

func (f *fakeSvc) Active(string) ([]huddle.HuddleView, error) { return []huddle.HuddleView{}, f.err }

func (f *fakeSvc) Start(_ context.Context, _, ch string) (huddle.StartResult, error) {
	return huddle.StartResult{Huddle: huddle.HuddleView{ID: "h1", ChannelID: ch}, MeetURL: "https://meet.test/abc", AlreadyRunning: true}, f.err
}

func (f *fakeSvc) Join(_ context.Context, user, id string) (string, error) {
	f.joined = append(f.joined, user+":"+id)
	if f.joinedErr != nil {
		return "", f.joinedErr
	}
	return "https://meet.test/abc", f.err
}
func (f *fakeSvc) Invite(context.Context, string, string, []string) error { return f.err }
func (f *fakeSvc) End(context.Context, string, string) error              { return f.err }
func (f *fakeSvc) RespondRing(context.Context, string, string, string, string) (string, error) {
	return f.huddleID, f.err
}

func do(t *testing.T, svc Service, method, path, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(stubAPI{}, svc)
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		r.Header.Set("Mattermost-User-Id", user)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(nil, w, r)
	return w
}

func TestAPIRequiresAuth(t *testing.T) {
	require.Equal(t, http.StatusUnauthorized, do(t, &fakeSvc{}, "GET", "/api/v1/huddles", "", "").Code)
}

func TestAPIStartReturnsMeetURL(t *testing.T) {
	w := do(t, &fakeSvc{}, "POST", "/api/v1/huddles", "u1", `{"channel_id":"c1"}`)
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Huddle         huddle.HuddleView `json:"huddle"`
		MeetURL        string            `json:"meet_url"`
		AlreadyRunning bool              `json:"already_running"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, "c1", got.Huddle.ChannelID)
	require.Equal(t, "https://meet.test/abc", got.MeetURL)
	require.True(t, got.AlreadyRunning)
}

func TestAPIRoutes(t *testing.T) {
	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/api/v1/huddles", "", 200},
		{"POST", "/api/v1/huddles/h1/join", "", 200},
		{"POST", "/api/v1/huddles/h1/invite", `{"user_ids":["u2"]}`, 204},
		{"POST", "/api/v1/huddles/h1/end", "", 204},
		{"POST", "/api/v1/rings/r1", `{"action":"decline"}`, 200},
		{"POST", "/api/v1/huddles", `{bad`, 400},
		{"GET", "/api/v1/huddles/h1/end", "", 404},
	} {
		require.Equal(t, c.want, do(t, &fakeSvc{}, c.method, c.path, "u1", c.body).Code, c.method+" "+c.path)
	}
}

func TestAPIRingAcceptJoins(t *testing.T) {
	svc := &fakeSvc{huddleID: "h9"}
	w := do(t, svc, "POST", "/api/v1/rings/r1", "u2", `{"action":"accept"}`)
	require.Equal(t, 200, w.Code)
	require.JSONEq(t, `{"meet_url":"https://meet.test/abc"}`, w.Body.String())
	require.Equal(t, []string{"u2:h9"}, svc.joined)

	svc = &fakeSvc{huddleID: "h9", joinedErr: huddle.ErrEnded}
	require.Equal(t, http.StatusGone, do(t, svc, "POST", "/api/v1/rings/r1", "u2", `{"action":"accept"}`).Code)
}

func TestAPIErrorMapping(t *testing.T) {
	for err, want := range map[error]int{
		huddle.ErrForbidden:                            403,
		huddle.ErrEnded:                                410,
		huddle.ErrNoOwner:                              422,
		huddle.ErrRateLimited:                          429,
		huddle.ErrNotFound:                             404,
		huddle.ErrBadRequest:                           400,
		fmt.Errorf("x: %w", huddle.ErrBadRequest):      400,
		huddle.ErrNotMember:                            400,
		huddle.ErrMeetUnavailable:                      503,
		errors.New("boom"):                             500,
		fmt.Errorf("wrapped: %w", huddle.ErrForbidden): 403,
	} {
		w := do(t, &fakeSvc{err: err}, "POST", "/api/v1/huddles/h1/end", "u1", "")
		require.Equal(t, want, w.Code, err.Error())
		var body struct{ Error string }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotEmpty(t, body.Error)
		require.NotContains(t, body.Error, "boom")
	}
}

func action(t *testing.T, svc *fakeSvc, header, userID string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(model.PostActionIntegrationRequest{UserId: userID, Context: map[string]any{"huddle_id": "h1"}})
	return do(t, svc, "POST", "/api/v1/actions/join", header, string(b))
}

func TestAPIActionJoinGotoLocation(t *testing.T) {
	svc := &fakeSvc{}
	w := action(t, svc, "u1", "u1")
	require.Equal(t, 200, w.Code)
	var resp model.PostActionIntegrationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "https://meet.test/abc", resp.GotoLocation)
	require.Equal(t, []string{"u1:h1"}, svc.joined)
}

func TestAPIActionJoinEndedEphemeral(t *testing.T) {
	w := action(t, &fakeSvc{err: huddle.ErrEnded}, "u1", "u1")
	require.Equal(t, 200, w.Code)
	var resp model.PostActionIntegrationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, huddle.Text("en", "huddle_ended"), resp.EphemeralText)
	require.Empty(t, resp.GotoLocation)
}

func TestAPIActionJoinUserMismatch403(t *testing.T) {
	svc := &fakeSvc{}
	require.Equal(t, 403, action(t, svc, "u1", "u2").Code)
	require.Empty(t, svc.joined)
}

func TestAPIActionJoinOtherErrorEphemeral(t *testing.T) {
	w := action(t, &fakeSvc{err: huddle.ErrForbidden}, "u1", "u1")
	require.Equal(t, 200, w.Code)
	var resp model.PostActionIntegrationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	want, _ := huddle.ErrorText("en", huddle.ErrForbidden)
	require.Equal(t, want, resp.EphemeralText)
}
