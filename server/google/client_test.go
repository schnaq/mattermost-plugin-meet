package google

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

const dummyKey = `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","private_key":"x","token_uri":"http://invalid"}`

func newTest(t *testing.T, h http.HandlerFunc) Meet {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	m, err := New([]byte(dummyKey), option.WithEndpoint(srv.URL), option.WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	return m
}

func TestCreateSpaceSendsOpenWithoutArtifactConfig(t *testing.T) {
	var body string
	m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		assert.Equal(t, "/v2/spaces", r.URL.Path)
		_, _ = w.Write([]byte(`{"name":"spaces/a","meetingUri":"https://meet.google.com/x"}`))
	})
	sp, err := m.CreateSpace(context.Background(), "o@x.de")
	require.NoError(t, err)
	assert.Equal(t, Space{Name: "spaces/a", MeetingURI: "https://meet.google.com/x"}, sp)
	assert.Contains(t, body, `"accessType":"OPEN"`)
	assert.NotContains(t, body, "artifactConfig")
}

func TestActiveParticipantsFilter(t *testing.T) {
	m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "latest_end_time IS NULL", r.URL.Query().Get("filter"))
		_, _ = w.Write([]byte(`{"participants":[
			{"signedinUser":{"user":"users/1","displayName":"Alice"}},
			{"anonymousUser":{"displayName":"123"}},
			{"phoneUser":{"displayName":"+49"}}]}`))
	})
	ps, err := m.ActiveParticipants(context.Background(), "o@x.de", "conferenceRecords/c")
	require.NoError(t, err)
	assert.Equal(t, []Participant{{"users/1", "Alice", false}, {"", "123", true}, {"", "+49", true}}, ps)
}

func TestEndActiveConferenceNoActiveIsNil(t *testing.T) {
	m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"There is no active conference for the given space."}}`))
	})
	assert.NoError(t, m.EndActiveConference(context.Background(), "o@x.de", "spaces/a"))
}

func TestEndActiveConferenceOther400IsError(t *testing.T) {
	m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"boom"}}`))
	})
	assert.Error(t, m.EndActiveConference(context.Background(), "o@x.de", "spaces/a"))
}

func TestRestrictSpaceUpdateMask(t *testing.T) {
	var body string
	m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "config.accessType", r.URL.Query().Get("updateMask"))
		_, _ = w.Write([]byte(`{}`))
	})
	require.NoError(t, m.RestrictSpace(context.Background(), "o@x.de", "spaces/a"))
	assert.Contains(t, body, `"accessType":"RESTRICTED"`)
}

func TestArtifacts(t *testing.T) {
	m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/recordings"):
			_, _ = w.Write([]byte(`{"recordings":[{"name":"r1","driveDestination":{"exportUri":"https://d/r"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/transcripts"):
			_, _ = w.Write([]byte(`{"transcripts":[{"name":"t1","docsDestination":{"exportUri":"https://d/t"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/smartNotes"):
			_, _ = w.Write([]byte(`{"smartNotes":[{"name":"n1","docsDestination":{"exportUri":"https://d/n"}}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	as, err := m.Artifacts(context.Background(), "o@x.de", "conferenceRecords/c")
	require.NoError(t, err)
	assert.Equal(t, []Artifact{
		{"recording", "r1", "https://d/r"}, {"transcript", "t1", "https://d/t"}, {"smart_notes", "n1", "https://d/n"},
	}, as)
}

func TestLookupEmailDomainPublic(t *testing.T) {
	m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/admin/directory/v1/users/123", r.URL.Path)
		assert.Equal(t, "domain_public", r.URL.Query().Get("viewType"))
		_, _ = w.Write([]byte(`{"primaryEmail":"a@x.de"}`))
	})
	email, found, err := m.LookupEmail(context.Background(), "o@x.de", "users/123")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "a@x.de", email)
}

func TestLookupEmailNotFoundIsNotError(t *testing.T) {
	for _, code := range []int{404, 403} {
		m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
		})
		email, found, err := m.LookupEmail(context.Background(), "o@x.de", "users/9")
		assert.NoError(t, err)
		assert.False(t, found)
		assert.Empty(t, email)
	}
}

func TestIsNotFound(t *testing.T) {
	for code, want := range map[int]bool{404: true, 500: false, 429: false} {
		m := newTest(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":{"code":` + strconv.Itoa(code) + `,"message":"x"}}`))
		})
		_, err := m.GetSpace(context.Background(), "o@x.de", "spaces/a")
		assert.Error(t, err)
		assert.Equal(t, want, IsNotFound(err), code)
	}
	assert.False(t, IsNotFound(nil))
}
