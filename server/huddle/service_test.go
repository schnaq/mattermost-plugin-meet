package huddle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
	"github.com/schnaq/mattermost-plugin-meet/server/kv"
)

const testMeetURI = "https://meet.google.com/abc-defg-hij"

type env struct {
	s     *Service
	mm    *fakeMM
	meet  *google.Fake
	kv    kv.KV
	store Store
	ring  *fakeRinger
	pres  *fakePresence
	cfg   Config
	clock time.Time // returned by the service's now(); starts at t0
}

// newEnv: users u1 (mapped to u1@corp.test, de), u2, u3, ext (all en);
// channel c1 (open) with u1,u2,u3,ext; dm (direct) with u1,u2.
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{
		mm:    newFakeMM(),
		meet:  &google.Fake{Space: google.Space{Name: "spaces/abc", MeetingURI: testMeetURI}},
		ring:  &fakeRinger{},
		pres:  &fakePresence{},
		clock: t0,
		cfg: Config{
			GoogleAccounts: map[string]string{"u1": "u1@corp.test"},
			PluginURL:      "https://mm.test/plugins/com.schnaq.meet",
		},
	}
	e.kv = kv.NewMemClock(func() time.Time { return e.clock })
	e.store = NewStore(e.kv)
	e.mm.addUser("u1", "Anna", "de")
	e.mm.addUser("u2", "Ben", "en")
	e.mm.addUser("u3", "Cleo", "")
	e.mm.addUser("ext", "Eve", "en")
	e.mm.addChannel("c1", model.ChannelTypeOpen, "u1", "u2", "u3", "ext")
	e.mm.addChannel("dm", model.ChannelTypeDirect, "u1", "u2")
	e.s = e.service(e.store)
	return e
}

func (e *env) service(store Store) *Service {
	return newService(e.mm, e.meet, store, func() Config { return e.cfg }, func() time.Time { return e.clock }, e.ring, e.pres)
}

func (e *env) start(t *testing.T, user, channel string) StartResult {
	t.Helper()
	res, err := e.s.Start(context.Background(), user, channel)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return res
}

func participantIDs(v HuddleView) []string {
	var ids []string
	for _, p := range v.Participants {
		ids = append(ids, p.UserID)
	}
	slices.Sort(ids)
	return ids
}

func postJSON(t *testing.T, p *model.Post) string {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStartCreatesSpacePostAndHuddle(t *testing.T) {
	e := newEnv(t)
	res := e.start(t, "u1", "c1")

	if res.AlreadyRunning || res.MeetURL != testMeetURI+"?authuser=u1%40corp.test" {
		t.Fatalf("result = %+v", res)
	}
	if e.meet.Calls[0] != "CreateSpace u1@corp.test " {
		t.Fatalf("calls = %q", e.meet.Calls)
	}
	if len(e.mm.created) != 1 {
		t.Fatalf("posts = %d", len(e.mm.created))
	}
	p := e.mm.created[0]
	if p.UserId != "u1" || p.ChannelId != "c1" || p.Type != "custom_meet_huddle" || p.GetProp("huddle_id") != res.Huddle.ID {
		t.Fatalf("post = %+v", p)
	}
	if js := postJSON(t, p); strings.Contains(js, "meet.google.com") || strings.Contains(js, "abc-defg-hij") {
		t.Fatalf("meet link leaked into post: %s", js)
	}
	atts := p.Attachments()
	if len(atts) != 1 || len(atts[0].Actions) != 1 || !strings.Contains(atts[0].Text, "Anna") {
		t.Fatalf("attachments = %+v", atts)
	}
	act := atts[0].Actions[0]
	if act.Name != "Beitreten" || act.Integration.URL != "https://mm.test/plugins/com.schnaq.meet/api/v1/actions/join" ||
		act.Integration.Context["huddle_id"] != res.Huddle.ID {
		t.Fatalf("action = %+v integration = %+v", act, act.Integration)
	}

	h, err := e.store.Huddle("c1")
	if err != nil || h == nil {
		t.Fatalf("huddle: %v %v", h, err)
	}
	if h.ID != res.Huddle.ID || h.Space != "spaces/abc" || h.MeetingURI != testMeetURI || h.OwnerEmail != "u1@corp.test" ||
		h.StarterUserID != "u1" || h.PostID != p.Id || !h.StartedAt.Equal(t0) || !h.BoostUntil.Equal(t0.Add(60*time.Second)) {
		t.Fatalf("huddle = %+v", h)
	}
	if pp := h.Participants["u1"]; pp.State != StateJoining || pp.DisplayName != "Anna" || !pp.Since.Equal(t0) {
		t.Fatalf("starter = %+v", pp)
	}
	if res.Huddle.StarterID != "u1" || res.Huddle.StartedAt != t0.UnixMilli() || len(res.Huddle.Participants) != 1 {
		t.Fatalf("view = %+v", res.Huddle)
	}

	if len(e.mm.ws) != 1 || e.mm.ws[0].Event != "huddle_changed" || e.mm.ws[0].Broadcast.ChannelId != "c1" {
		t.Fatalf("ws = %+v", e.mm.ws)
	}
	var v HuddleView
	if err := json.Unmarshal([]byte(e.mm.ws[0].Payload["huddle"].(string)), &v); err != nil || v.ID != h.ID {
		t.Fatalf("ws payload: %v %+v", err, v)
	}
	if len(e.ring.calls) != 0 {
		t.Fatalf("open channel must not ring: %q", e.ring.calls)
	}
}

func TestStartTwiceJoinsExisting(t *testing.T) {
	e := newEnv(t)
	first := e.start(t, "u1", "c1")
	// ext has no Google account and there is no fallback owner: joining must still work.
	res := e.start(t, "ext", "c1")

	if !res.AlreadyRunning || res.Huddle.ID != first.Huddle.ID || res.MeetURL != testMeetURI {
		t.Fatalf("result = %+v", res)
	}
	if len(e.meet.Calls) != 1 || len(e.mm.created) != 1 {
		t.Fatalf("calls = %q, posts = %d", e.meet.Calls, len(e.mm.created))
	}
	assertStrings(t, "participants", participantIDs(res.Huddle), []string{"ext", "u1"})
	if last := e.mm.updated[len(e.mm.updated)-1]; !strings.Contains(last.Attachments()[0].Text, "Eve") {
		t.Fatalf("card not updated: %+v", last.Attachments()[0])
	}
}

func TestStartOwnerFallback(t *testing.T) {
	e := newEnv(t)
	e.cfg.FallbackOwnerEmail = "owner@corp.test"
	res := e.start(t, "u2", "c1")

	if e.meet.Calls[0] != "CreateSpace owner@corp.test " {
		t.Fatalf("calls = %q", e.meet.Calls)
	}
	if res.MeetURL != testMeetURI {
		t.Fatalf("unmapped user must not get authuser: %s", res.MeetURL)
	}
	if h, _ := e.store.Huddle("c1"); h.OwnerEmail != "owner@corp.test" {
		t.Fatalf("owner = %q", h.OwnerEmail)
	}
}

func TestStartNoOwner(t *testing.T) {
	e := newEnv(t)
	if _, err := e.s.Start(context.Background(), "u2", "c1"); !errors.Is(err, ErrNoOwner) {
		t.Fatalf("err = %v", err)
	}
	if len(e.meet.Calls) != 0 || len(e.mm.created) != 0 {
		t.Fatalf("calls = %q, posts = %d", e.meet.Calls, len(e.mm.created))
	}
}

func TestStartNonMemberForbidden(t *testing.T) {
	e := newEnv(t)
	if _, err := e.s.Start(context.Background(), "u3", "dm"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	if len(e.meet.Calls) != 0 || len(e.mm.created) != 0 {
		t.Fatalf("calls = %q, posts = %d", e.meet.Calls, len(e.mm.created))
	}
}

func TestStartGoogleErrorReturnsUserError(t *testing.T) {
	e := newEnv(t)
	e.meet.Err = map[string]error{"CreateSpace": errors.New("googleapi: Error 403: delegation denied for u1@corp.test")}
	_, err := e.s.Start(context.Background(), "u1", "c1")
	if !errors.Is(err, ErrMeetUnavailable) || strings.Contains(err.Error(), "u1@corp.test") {
		t.Fatalf("err = %v", err)
	}
	if keys, _ := e.kv.ListKeys(""); len(keys) != 0 {
		t.Fatalf("kv keys = %q", keys)
	}
	if len(e.mm.created) != 0 || len(e.mm.ws) != 0 {
		t.Fatalf("posts = %d, ws = %d", len(e.mm.created), len(e.mm.ws))
	}
	if len(e.mm.logs) != 1 || !strings.Contains(e.mm.logs[0], "delegation denied") {
		t.Fatalf("logs = %q", e.mm.logs)
	}
}

func TestStartInDMRingsOthers(t *testing.T) {
	e := newEnv(t)
	res := e.start(t, "u1", "dm")
	want := "channel " + res.Huddle.ID + " " + e.mm.created[0].Id + " u1"
	assertStrings(t, "rings", e.ring.calls, []string{want})
}

// slowCreate holds every CreateSpace until n calls arrived, so concurrent Starts all
// pass the "already running?" check before any of them creates the huddle.
type slowCreate struct {
	*google.Fake
	wg *sync.WaitGroup
}

func (s slowCreate) CreateSpace(ctx context.Context, owner string) (google.Space, error) {
	s.wg.Done()
	s.wg.Wait()
	return s.Fake.CreateSpace(ctx, owner)
}

func TestStartConcurrentCreatesOneHuddle(t *testing.T) {
	e := newEnv(t)
	e.cfg.GoogleAccounts["u2"] = "u2@corp.test"
	wg := &sync.WaitGroup{}
	wg.Add(2)
	s := newService(e.mm, slowCreate{e.meet, wg}, e.store, func() Config { return e.cfg }, func() time.Time { return e.clock }, e.ring, e.pres)

	var res [2]StartResult
	var errs [2]error
	var done sync.WaitGroup
	for i, u := range []string{"u1", "u2"} {
		done.Go(func() { res[i], errs[i] = s.Start(context.Background(), u, "c1") })
	}
	done.Wait()

	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("errs = %v", errs)
	}
	if res[0].Huddle.ID != res[1].Huddle.ID || res[0].AlreadyRunning == res[1].AlreadyRunning {
		t.Fatalf("results = %+v", res)
	}
	if hs, _, _ := e.store.ActiveHuddles(); len(hs) != 1 || len(hs[0].Participants) != 2 {
		t.Fatalf("active = %+v", hs)
	}
	if len(e.mm.created) != 1 {
		t.Fatalf("card posts = %d", len(e.mm.created))
	}
	card := e.mm.posts[e.mm.created[0].Id].Attachments()[0].Text
	if !strings.Contains(card, "Anna") || !strings.Contains(card, "Ben") {
		t.Fatalf("final card = %q", card)
	}
	if !slices.ContainsFunc(e.mm.logs, func(l string) bool { return strings.Contains(l, "abandon") && strings.Contains(l, "spaces/abc") }) {
		t.Fatalf("logs = %q", e.mm.logs)
	}
	if n := len(slices.DeleteFunc(slices.Clone(e.meet.Calls), func(c string) bool { return !strings.HasPrefix(c, "RestrictSpace ") })); n != 1 {
		t.Fatalf("abandoned space must be restricted once: %q", e.meet.Calls)
	}
}

func TestMeetURLAuthUserOnlyForMappedUsers(t *testing.T) {
	e := newEnv(t)
	e.cfg.GoogleAccounts["u2"] = "b+x@corp.test"
	h := &Huddle{MeetingURI: testMeetURI}
	if got := e.s.meetURL(h, "u2"); got != testMeetURI+"?authuser=b%2Bx%40corp.test" {
		t.Fatalf("mapped = %s", got)
	}
	if got := e.s.meetURL(h, "u3"); got != testMeetURI {
		t.Fatalf("unmapped = %s", got)
	}
	e.cfg.GoogleAccounts["u3"] = ""
	if got := e.s.meetURL(h, "u3"); got != testMeetURI {
		t.Fatalf("empty mapping = %s", got)
	}
}

func TestJoinAddsJoiningParticipant(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	url, err := e.s.Join(context.Background(), "u2", id)
	if err != nil || url != testMeetURI {
		t.Fatalf("join: %q %v", url, err)
	}
	h, _ := e.store.Huddle("c1")
	if p := h.Participants["u2"]; p.State != StateJoining || p.DisplayName != "Ben" {
		t.Fatalf("u2 = %+v", p)
	}
	// start refreshes the card once after storing PostID, the join once more
	if len(e.mm.ws) != 2 || len(e.mm.updated) != 2 || !strings.Contains(e.mm.updated[1].Attachments()[0].Text, "Ben") {
		t.Fatalf("ws = %d, updates = %d", len(e.mm.ws), len(e.mm.updated))
	}
}

func TestJoinKeepsConfirmedParticipant(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	confirmed := Participant{UserID: "u1", DisplayName: "Anna", State: StateConfirmed, Since: t0.Add(-time.Minute), MissingSince: t0.Add(-5 * time.Second)}
	_, _ = e.store.UpdateHuddle("c1", func(h *Huddle) error { h.Participants["u1"] = confirmed; return nil })

	if _, err := e.s.Join(context.Background(), "u1", id); err != nil {
		t.Fatal(err)
	}
	h, _ := e.store.Huddle("c1")
	if p := h.Participants["u1"]; p != confirmed {
		t.Fatalf("confirmed participant changed: %+v", p)
	}
}

func TestJoinEndedHuddle(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	h, _ := e.store.Huddle("c1")
	_ = e.store.EndHuddle(h)
	if _, err := e.s.Join(context.Background(), "u2", id); !errors.Is(err, ErrEnded) {
		t.Fatalf("err = %v", err)
	}
	if _, err := e.s.Join(context.Background(), "u2", "unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

func TestJoinRejectedAfterRemovalFromChannel(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.mm.removeMember("c1", "u2")
	if _, err := e.s.Join(context.Background(), "u2", id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	if h, _ := e.store.Huddle("c1"); len(h.Participants) != 1 {
		t.Fatalf("participants = %+v", h.Participants)
	}
}

func TestJoinDuringTickKeepsBoth(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	tick := func() {
		_, _ = NewStore(e.kv).UpdateHuddle("c1", func(h *Huddle) error {
			p := h.Participants["u1"]
			p.State = StateConfirmed
			h.Participants["u1"] = p
			h.ConferenceRecords = []string{testRecord}
			return nil
		})
	}
	s := e.service(NewStore(&interferingKV{KV: e.kv, inject: tick}))
	if _, err := s.Join(context.Background(), "u2", id); err != nil {
		t.Fatal(err)
	}
	h, _ := e.store.Huddle("c1")
	if h.Participants["u1"].State != StateConfirmed || len(h.ConferenceRecords) == 0 || h.Participants["u2"].State != StateJoining {
		t.Fatalf("huddle = %+v", h)
	}
}

func TestInviteRequiresMembership(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "dm").Huddle.ID
	e.ring.calls = nil
	if err := e.s.Invite(context.Background(), "u1", id, []string{"u2", "u3"}); !errors.Is(err, ErrNotMember) || !errors.Is(err, ErrBadRequest) {
		t.Fatalf("non-member target: err = %v", err)
	}
	if msg, unexpected := ErrorText("de", fmt.Errorf("x: %w", ErrNotMember)); unexpected || msg != Text("de", "err_not_member") {
		t.Fatalf("text = %q", msg)
	}
	if err := e.s.Invite(context.Background(), "u3", id, []string{"u2"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-member inviter: err = %v", err)
	}
	if len(e.ring.calls) != 0 {
		t.Fatalf("rings = %q", e.ring.calls)
	}
	// The rejected call must not have burned u2's rate limit.
	if err := e.s.Invite(context.Background(), "u1", id, []string{"u2"}); err != nil {
		t.Fatalf("valid invite: %v", err)
	}
}

func TestInviteRateLimited(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	ctx := context.Background()
	if err := e.s.Invite(ctx, "u1", id, []string{"u2"}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Invite(ctx, "u1", id, []string{"u2"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	if err := e.s.Invite(ctx, "u1", id, []string{"u2", "u3"}); err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "rings", e.ring.calls, []string{"users " + id + " u1 [u2]", "users " + id + " u1 [u3]"})
}

func TestEndOnlyStarter(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	ctx := context.Background()
	if err := e.s.End(ctx, "u2", id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	if err := e.s.End(ctx, "u1", id); err != nil {
		t.Fatal(err)
	}
	if len(e.meet.Calls) != 1 { // only CreateSpace: the tick ends the conference
		t.Fatalf("calls = %q", e.meet.Calls)
	}
}

func TestEndSetsEndingSinceNotEnding(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	if err := e.s.End(context.Background(), "u1", id); err != nil {
		t.Fatal(err)
	}
	h, _ := e.store.Huddle("c1")
	if h == nil || !h.EndingSince.Equal(t0) || h.Ending || !h.NextPollAt.Equal(t0) || !h.BoostUntil.Equal(t0.Add(boostFor)) {
		t.Fatalf("huddle = %+v", h)
	}
	last := e.mm.ws[len(e.mm.ws)-1].Payload["huddle"].(string)
	if !strings.Contains(last, `"ending":false`) || !strings.Contains(last, `"end_requested":true`) {
		t.Fatalf("ws = %s", last)
	}
}

func TestEndGoogleFailureStillFlagsHuddle(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.meet.Err = map[string]error{"EndActiveConference": errors.New("googleapi: 500 for u1@corp.test")}
	if err := e.s.End(context.Background(), "u1", id); err != nil {
		t.Fatalf("err = %v", err)
	}
	e.tick() // End polls right away: the tick ends the conference, which fails
	if n := e.countCalls("EndActiveConference"); n != 1 {
		t.Fatalf("calls = %q", e.meet.Calls)
	}
	if h := e.huddle(t); !h.EndingSince.Equal(t0) || h.Finishing {
		t.Fatalf("huddle = %+v", h)
	}
	if len(e.mm.logs) == 0 {
		t.Fatal("google error not logged")
	}
}

func TestJoinAfterEndIsEnded(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	ctx := context.Background()
	if err := e.s.End(ctx, "u1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Join(ctx, "u2", id); !errors.Is(err, ErrEnded) {
		t.Fatalf("join: err = %v", err)
	}
	if _, err := e.s.Start(ctx, "u2", "c1"); !errors.Is(err, ErrEnded) {
		t.Fatalf("start-into-join: err = %v", err)
	}
	if err := e.s.Invite(ctx, "u1", id, []string{"u2"}); !errors.Is(err, ErrEnded) {
		t.Fatalf("invite: err = %v", err)
	}
	if h, _ := e.store.Huddle("c1"); len(h.Participants) != 1 {
		t.Fatalf("participants = %+v", h.Participants)
	}
}

// casErrKV fails every CompareAndSet.
type casErrKV struct{ kv.KV }

func (casErrKV) CompareAndSet(string, any, any) (bool, error) { return false, errors.New("kv down") }

func TestStartStoreErrorRestrictsSpace(t *testing.T) {
	e := newEnv(t)
	s := e.service(NewStore(casErrKV{e.kv}))
	if _, err := s.Start(context.Background(), "u1", "c1"); err == nil {
		t.Fatal("expected error")
	}
	assertStrings(t, "calls", e.meet.Calls, []string{"CreateSpace u1@corp.test ", "RestrictSpace u1@corp.test spaces/abc"})
	if len(e.mm.created) != 0 || !slices.ContainsFunc(e.mm.logs, func(l string) bool { return strings.Contains(l, "spaces/abc") }) {
		t.Fatalf("posts = %d, logs = %q", len(e.mm.created), e.mm.logs)
	}
}

func TestStartArchivedChannelForbidden(t *testing.T) {
	e := newEnv(t)
	e.mm.channels["c1"].DeleteAt = 1
	if _, err := e.s.Start(context.Background(), "u1", "c1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	if len(e.meet.Calls) != 0 {
		t.Fatalf("calls = %q", e.meet.Calls)
	}
}

func TestInviteSkipsSelfBotsAndDeactivated(t *testing.T) {
	e := newEnv(t)
	e.mm.addUser("bot", "Bot", "en")
	e.mm.users["bot"].IsBot = true
	e.mm.addUser("gone", "Gone", "en")
	e.mm.users["gone"].DeleteAt = 1
	e.mm.addChannel("c2", model.ChannelTypeOpen, "u1", "u2", "bot", "gone")
	id := e.start(t, "u1", "c2").Huddle.ID
	ctx := context.Background()

	if err := e.s.Invite(ctx, "u1", id, []string{"u1", "bot", "gone"}); err != nil {
		t.Fatalf("only skipped targets: %v", err)
	}
	if err := e.s.Invite(ctx, "u1", id, []string{"u2", "bot", "gone", "u1"}); err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "rings", e.ring.calls, []string{"users " + id + " u1 [u2]"})
	for _, to := range []string{"u1", "bot", "gone"} {
		if !e.store.TakeRateLimit("u1", to) {
			t.Fatalf("skipped target %s took a rate-limit slot", to)
		}
	}
}

func TestActiveFiltersByMembership(t *testing.T) {
	e := newEnv(t)
	e.start(t, "u1", "c1")
	dm := e.start(t, "u1", "dm")
	views, err := e.s.Active("u2")
	if err != nil || len(views) != 2 {
		t.Fatalf("u2: %v %+v", err, views)
	}
	e.mm.removeMember("c1", "u2")
	views, _ = e.s.Active("u2")
	if len(views) != 1 || views[0].ID != dm.Huddle.ID {
		t.Fatalf("u2 after removal: %+v", views)
	}
	if views, _ = e.s.Active("u3"); len(views) != 1 || views[0].ChannelID != "c1" {
		t.Fatalf("u3: %+v", views)
	}
}

func TestTextLocale(t *testing.T) {
	if Text("de-DE", "join") != "Beitreten" || Text("en", "join") != "Join" || Text("fr", "join") != "Join" || Text("", "join") != "Join" {
		t.Fatal("locale fallback broken")
	}
}

func TestErrorText(t *testing.T) {
	if msg, unexpected := ErrorText("de", fmt.Errorf("x: %w", ErrBadRequest)); unexpected || msg != "Ungültige Anfrage." {
		t.Fatalf("wrapped sentinel: %q %v", msg, unexpected)
	}
	if msg, unexpected := ErrorText("en", errors.New("db down")); !unexpected || msg != "Something went wrong." {
		t.Fatalf("unknown error: %q %v", msg, unexpected)
	}
}

func TestJoinWithDeletedCardOnlyLogs(t *testing.T) {
	e := newEnv(t)
	res := e.start(t, "u1", "c1")
	e.mm.deletePost(e.mm.created[0].Id)
	if _, err := e.s.Join(context.Background(), "u2", res.Huddle.ID); err != nil {
		t.Fatalf("join must not fail on a deleted card: %v", err)
	}
	if len(e.mm.logs) != 1 || !strings.Contains(e.mm.logs[0], "card update failed") || len(e.mm.ws) != 2 {
		t.Fatalf("logs = %q, ws = %d", e.mm.logs, len(e.mm.ws))
	}
}

func TestStartIDWriteFailureLeavesNoHuddle(t *testing.T) {
	e := newEnv(t)
	fkv := &failSetKV{KV: e.kv, prefix: "hid:", fail: true}
	e.s = e.service(NewStore(fkv))
	if _, err := e.s.Start(context.Background(), "u1", "c1"); err == nil {
		t.Fatal("expected error")
	}
	if h, err := e.store.Huddle("c1"); h != nil || err != nil {
		t.Fatalf("dead huddle left behind: %+v %v", h, err)
	}
	if e.countCalls("RestrictSpace") != 1 {
		t.Fatalf("calls = %q", e.meet.Calls)
	}
	fkv.fail = false
	if res := e.start(t, "u1", "c1"); res.AlreadyRunning {
		t.Fatalf("res = %+v", res)
	}
}

func TestStartLostRaceDropsIDEntry(t *testing.T) {
	e := newEnv(t)
	winner := &Huddle{ID: "winner", ChannelID: "c1"}
	if ok, err := e.store.CreateHuddle(winner); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := e.store.CreateHuddle(&Huddle{ID: "loser", ChannelID: "c1"}); ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, err := e.store.HuddleByID("loser"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("loser id: err = %v", err)
	}
}

func TestStartRangOnlyForNewDM(t *testing.T) {
	e := newEnv(t)
	if !e.start(t, "u1", "dm").Rang {
		t.Fatal("new DM huddle: Rang = false")
	}
	if e.start(t, "u2", "dm").Rang {
		t.Fatal("joined DM huddle: Rang = true")
	}
	if e.start(t, "u1", "c1").Rang {
		t.Fatal("open channel: Rang = true")
	}
}

func TestInviteFailedRingKeepsNoRateLimit(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	ctx := context.Background()
	e.ring.fail = map[string]error{"u2": errors.New("dm failed")}
	// u3 was rung: a partial failure is no error for the inviter, only logged.
	logs := len(e.mm.logs)
	if err := e.s.Invite(ctx, "u1", id, []string{"u2", "u3"}); err != nil || len(e.mm.logs) != logs+1 {
		t.Fatalf("partial failure: err = %v logs = %q", err, e.mm.logs[logs:])
	}
	if err := e.s.Invite(ctx, "u1", id, []string{"u2"}); err == nil {
		t.Fatal("nobody rung: expected the ring error")
	}
	e.ring.fail = nil
	if err := e.s.Invite(ctx, "u1", id, []string{"u2"}); err != nil {
		t.Fatalf("retry after failed ring: %v", err)
	}
	if err := e.s.Invite(ctx, "u1", id, []string{"u3"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("delivered ring must stay limited: %v", err)
	}
}

func TestInviteUnknownTargetAndMembershipOrder(t *testing.T) {
	e := newEnv(t)
	id := e.start(t, "u1", "dm").Huddle.ID
	ctx := context.Background()
	if err := e.s.Invite(ctx, "u1", id, []string{"nobody"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("unknown target: err = %v", err)
	}
	if err := e.s.Invite(ctx, "u3", id, []string{"nobody"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-member inviter: err = %v", err)
	}
}
