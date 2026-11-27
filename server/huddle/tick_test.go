package huddle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"google.golang.org/api/googleapi"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
	"github.com/schnaq/mattermost-plugin-meet/server/kv"
)

const testRecord = "conferenceRecords/r1"

var (
	gAnna  = google.Participant{GoogleUserID: "users/1", DisplayName: "Anna Meet"}
	gBen   = google.Participant{GoogleUserID: "users/2", DisplayName: "Ben Meet"}
	gExt   = google.Participant{GoogleUserID: "users/999", DisplayName: "Xavier"}
	gGuest = google.Participant{DisplayName: "Gast", Anonymous: true}
)

// newTickEnv: like newEnv, u2 also mapped; Directory knows users/1 and users/2; bot "bot".
func newTickEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.cfg.GoogleAccounts["u2"] = "u2@corp.test"
	e.cfg.BotUserID = "bot"
	e.meet.Emails = map[string]string{"users/1": "U1@corp.test", "users/2": "u2@corp.test"}
	return e
}

func (e *env) tick() { e.s.Tick(context.Background()) }

func (e *env) advance(d time.Duration) { e.clock = e.clock.Add(d) }

// live makes the conference active with the given participants.
func (e *env) live(ps ...google.Participant) {
	e.meet.Space.ActiveConference = testRecord
	e.meet.Participants = ps
}

// endConference makes the conference disappear and ticks until endGrace is over: the huddle ends.
func (e *env) endConference() {
	e.meet.Space.ActiveConference = ""
	e.tick() // gone: GoneSince is set, the huddle still runs
	e.advance(endGrace)
	e.tick()
}

func (e *env) huddle(t *testing.T) *Huddle {
	t.Helper()
	h, err := e.store.Huddle("c1")
	if err != nil || h == nil {
		t.Fatalf("huddle: %v %v", h, err)
	}
	return h
}

func (e *env) countCalls(method string) int {
	n := 0
	for _, c := range e.meet.Calls {
		if strings.HasPrefix(c, method+" ") {
			n++
		}
	}
	return n
}

func (e *env) lastView(t *testing.T) HuddleView {
	t.Helper()
	if len(e.mm.ws) == 0 {
		t.Fatal("no websocket event")
	}
	ev := e.mm.ws[len(e.mm.ws)-1]
	var v HuddleView
	if err := json.Unmarshal([]byte(ev.Payload["huddle"].(string)), &v); ev.Event != "huddle_changed" || err != nil {
		t.Fatalf("event = %+v err = %v", ev, err)
	}
	return v
}

func (e *env) assertEnded(t *testing.T, id string) {
	t.Helper()
	if h, err := e.store.HuddleByID(id); h != nil || !errors.Is(err, ErrEnded) {
		t.Fatalf("HuddleByID = %v %v", h, err)
	}
}

func (e *env) followup(t *testing.T) *Followup {
	t.Helper()
	_, _, fs, err := e.store.TickState()
	if err != nil || len(fs) > 1 {
		t.Fatalf("followups = %v %v", fs, err)
	}
	if len(fs) == 0 {
		return nil
	}
	return fs[0]
}

func TestTickConfirmsParticipantsAndUpdatesCard(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna, gGuest)
	updates, events := len(e.mm.updated), len(e.mm.ws)

	e.tick()

	h := e.huddle(t)
	if !slices.Equal(h.ConferenceRecords, []string{testRecord}) || h.Ending {
		t.Fatalf("huddle = %+v", h)
	}
	if h.Participants["u1"].State != StateConfirmed || h.Participants["a:Gast"].State != StateConfirmed {
		t.Fatalf("participants = %+v", h.Participants)
	}
	if !h.NextPollAt.Equal(t0.Add(4 * time.Second)) {
		t.Fatalf("next poll = %v", h.NextPollAt)
	}
	if uid, _ := e.store.GoogleMapping("users/1"); uid != "u1" {
		t.Fatalf("gid cache = %q", uid)
	}
	if !slices.Equal(e.pres.calls, []string{"joined u1 " + id}) {
		t.Fatalf("presence = %q", e.pres.calls)
	}
	if len(e.mm.updated) != updates+1 || !strings.Contains(e.mm.updated[updates].Attachments()[0].Text, "Gast") {
		t.Fatalf("card updates = %d", len(e.mm.updated)-updates)
	}
	if len(e.mm.ws) != events+1 || len(e.lastView(t).Participants) != 2 {
		t.Fatalf("ws = %+v", e.mm.ws[events:])
	}
	if !slices.Contains(e.ring.calls, "expire "+t0.Format(time.RFC3339)+" 0") {
		t.Fatalf("ring calls = %q", e.ring.calls)
	}

	// Nothing visible changed: no card update, no event.
	e.advance(4 * time.Second)
	e.tick()
	if len(e.mm.updated) != updates+1 || len(e.mm.ws) != events+1 || len(e.pres.calls) != 1 {
		t.Fatalf("unchanged tick produced updates=%d ws=%d presence=%q", len(e.mm.updated)-updates, len(e.mm.ws)-events, e.pres.calls)
	}
}

func TestTickEndsWhenConferenceGone(t *testing.T) {
	e := newTickEnv(t)
	res := e.start(t, "u1", "c1")
	id := res.Huddle.ID
	e.live(gAnna)
	e.tick()
	postID := e.huddle(t).PostID

	e.advance(30*time.Minute - endGrace)
	e.endConference()

	e.assertEnded(t, id)
	if !slices.Contains(e.meet.Calls, "RestrictSpace u1@corp.test spaces/abc") {
		t.Fatalf("calls = %q", e.meet.Calls)
	}
	if !slices.Contains(e.pres.calls, "left u1 "+id) {
		t.Fatalf("presence = %q", e.pres.calls)
	}
	if v := e.lastView(t); !v.Ended || v.ID != id {
		t.Fatalf("view = %+v", v)
	}
	card := e.mm.posts[postID]
	atts := card.Attachments()
	if card.GetProp("ended") != true || !strings.Contains(card.Message, "30 min") || !strings.Contains(card.Message, "beendet") ||
		len(atts) != 1 || len(atts[0].Actions) != 0 || !strings.Contains(atts[0].Text, "Anna") {
		t.Fatalf("summary = %+v attachments = %+v", card, atts)
	}
	f := e.followup(t)
	if f == nil || !slices.Equal(f.ConferenceRecords, []string{testRecord}) || f.PostID != postID || f.ChannelID != "c1" || f.OwnerEmail != "u1@corp.test" ||
		f.Locale != "de" || !f.Until.Equal(e.clock.Add(2*time.Hour)) || !f.NextAt.Equal(e.clock.Add(5*time.Minute)) {
		t.Fatalf("followup = %+v", f)
	}

	// Ended huddles are not polled again.
	calls := len(e.meet.Calls)
	e.advance(time.Minute)
	e.tick()
	if len(e.meet.Calls) != calls {
		t.Fatalf("calls after end = %q", e.meet.Calls[calls:])
	}
}

func TestTickEndsWhenNobodyJoinedIn10Min(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID

	e.advance(10 * time.Minute)
	e.tick()
	e.huddle(t) // still running at exactly 10 min

	e.advance(15 * time.Second)
	e.tick()
	e.assertEnded(t, id)
	if !slices.Contains(e.meet.Calls, "RestrictSpace u1@corp.test spaces/abc") || e.followup(t) != nil {
		t.Fatalf("calls = %q followup = %+v", e.meet.Calls, e.followup(t))
	}
	if !slices.Contains(e.pres.calls, "left u1 "+id) {
		t.Fatalf("presence = %q", e.pres.calls)
	}
}

func TestTickEndsAfter12h(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()

	e.advance(12*time.Hour + time.Second)
	e.tick()

	e.assertEnded(t, id)
	if f := e.followup(t); f == nil || !slices.Equal(f.ConferenceRecords, []string{testRecord}) {
		t.Fatalf("followup = %+v", f)
	}
}

func TestTickEndsWhenEndingBeforeConference(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	if err := e.s.End(context.Background(), "u1", id); err != nil {
		t.Fatal(err)
	}
	e.advance(time.Second)
	e.tick()
	e.assertEnded(t, id)
}

func TestTickUsesBoostInterval(t *testing.T) {
	e := newTickEnv(t)
	e.start(t, "u1", "c1")

	e.tick()
	if h := e.huddle(t); !h.NextPollAt.Equal(t0.Add(4 * time.Second)) {
		t.Fatalf("next poll = %v", h.NextPollAt)
	}
	e.advance(2 * time.Second)
	e.tick()
	if n := e.countCalls("GetSpace"); n != 1 {
		t.Fatalf("GetSpace calls = %d, want 1 (not due yet)", n)
	}

	e.clock = t0.Add(61 * time.Second)
	e.tick()
	if h := e.huddle(t); !h.NextPollAt.Equal(e.clock.Add(15 * time.Second)) {
		t.Fatalf("next poll = %v", h.NextPollAt)
	}
}

func TestTickGoogleErrorKeepsState(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	before := e.huddle(t)
	events := len(e.mm.ws)

	// Each case would end the huddle or mark u1 missing if the partial data were trusted.
	for method, conference := range map[string]string{"GetSpace": "", "ActiveParticipants": testRecord} {
		e.meet.Err = map[string]error{method: errors.New("boom")}
		e.meet.Space.ActiveConference, e.meet.Participants = conference, nil
		e.advance(time.Hour)
		logs := len(e.mm.logs)
		e.tick()

		h := e.huddle(t)
		if h.Participants["u1"].State != StateConfirmed || !h.Participants["u1"].MissingSince.IsZero() || h.Ending ||
			!slices.Equal(h.ConferenceRecords, before.ConferenceRecords) || !h.NextPollAt.Equal(e.clock.Add(15*time.Second)) {
			t.Fatalf("%s: huddle = %+v", method, h)
		}
		if len(e.mm.ws) != events || len(e.mm.logs) == logs {
			t.Fatalf("%s: ws = %d logs = %q", method, len(e.mm.ws)-events, e.mm.logs[logs:])
		}
	}

	// The 12 h cap still applies.
	e.meet.Err = map[string]error{"GetSpace": errors.New("boom")}
	e.clock = t0.Add(12*time.Hour + time.Second)
	e.tick()
	e.assertEnded(t, id)
}

func TestTickResumesHuddleLoadedFromKV(t *testing.T) {
	e := newTickEnv(t)
	h := &Huddle{
		ID: "h1", ChannelID: "c1", Space: "spaces/abc", MeetingURI: testMeetURI, OwnerEmail: "u1@corp.test",
		StarterUserID: "u1", PostID: "p1", StartedAt: t0.Add(-time.Hour), ConferenceRecords: []string{testRecord},
		SeenUserIDs:  []string{"u1"},
		Participants: map[string]Participant{"u1": {UserID: "u1", DisplayName: "Anna", State: StateConfirmed, Since: t0.Add(-time.Hour)}},
	}
	if ok, err := e.store.CreateHuddle(h); !ok || err != nil {
		t.Fatalf("seed: %v %v", ok, err)
	}
	e.s = e.service(NewStore(e.kv)) // fresh service, no in-memory state

	e.endConference()

	e.assertEnded(t, "h1")
	if !slices.Contains(e.pres.calls, "left u1 h1") || e.followup(t) == nil {
		t.Fatalf("presence = %q followup = %+v", e.pres.calls, e.followup(t))
	}
}

func TestTickIgnoresDeletedPost(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.mm.deletePost(e.huddle(t).PostID)
	e.live(gAnna, gGuest)

	e.tick()
	if h := e.huddle(t); h.Participants["a:Gast"].State != StateConfirmed {
		t.Fatalf("participants = %+v", h.Participants)
	}
	e.advance(time.Minute)
	e.endConference()
	e.assertEnded(t, id)
	if !slices.ContainsFunc(e.mm.logs, func(l string) bool { return strings.Contains(l, "card update failed") }) {
		t.Fatalf("logs = %q", e.mm.logs)
	}
}

func TestTickExternalGoogleUserDoesNotFreezePresence(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gExt, gAnna, gBen)

	e.tick()

	h := e.huddle(t)
	for _, k := range []string{"u1", "u2", "g:users/999"} {
		if h.Participants[k].State != StateConfirmed {
			t.Fatalf("%s not confirmed: %+v", k, h.Participants)
		}
	}
	if h.Participants["u2"].DisplayName != "Ben" {
		t.Fatalf("u2 name = %q", h.Participants["u2"].DisplayName)
	}
	if !slices.Equal(e.pres.calls, []string{"joined u1 " + id, "joined u2 " + id}) {
		t.Fatalf("presence = %q", e.pres.calls)
	}

	// Guests leaving never reach presence.
	e.meet.Participants = []google.Participant{gAnna, gBen}
	e.advance(time.Minute)
	e.tick()
	e.advance(time.Minute)
	e.tick()
	if _, ok := e.huddle(t).Participants["g:users/999"]; ok || len(e.pres.calls) != 2 {
		t.Fatalf("participants = %+v presence = %q", e.huddle(t).Participants, e.pres.calls)
	}
}

func TestTickCachesDirectoryLookups(t *testing.T) {
	e := newTickEnv(t)
	e.start(t, "u1", "c1")
	e.live(gAnna, gExt)

	e.tick()
	e.advance(5 * time.Second)
	e.tick()
	if n := e.countCalls("LookupEmail"); n != 2 {
		t.Fatalf("LookupEmail calls = %d, want 2 (one per user): %q", n, e.meet.Calls)
	}

	// The negative entry for the external user expires after an hour; the positive one after a day.
	e.advance(time.Hour)
	e.tick()
	if !slices.Equal(lookups(e), []string{"users/1", "users/999", "users/999"}) {
		t.Fatalf("lookups = %q", lookups(e))
	}

	// Directory errors are not cached.
	e.meet.Err = map[string]error{"LookupEmail": errors.New("boom")}
	e.advance(time.Hour)
	e.tick()
	e.meet.Err = nil
	e.advance(15 * time.Second)
	e.tick()
	if n := e.countCalls("LookupEmail"); n != 5 {
		t.Fatalf("LookupEmail calls = %d, want 5: %q", n, lookups(e))
	}
}

func lookups(e *env) []string {
	var out []string
	for _, c := range e.meet.Calls {
		if strings.HasPrefix(c, "LookupEmail ") {
			out = append(out, c[strings.LastIndex(c, " ")+1:])
		}
	}
	return out
}

func TestTickZeroParticipantsMarksEnding(t *testing.T) {
	e := newTickEnv(t)
	e.start(t, "u1", "c1")
	e.live(gAnna)
	e.tick()

	e.meet.Participants = nil
	e.advance(5 * time.Second)
	e.tick()
	if h := e.huddle(t); !h.Ending || h.Participants["u1"].MissingSince.IsZero() {
		t.Fatalf("huddle = %+v", h)
	}
	if !e.lastView(t).Ending {
		t.Fatalf("view = %+v", e.lastView(t))
	}

	e.meet.Participants = []google.Participant{gAnna}
	e.advance(5 * time.Second)
	e.tick()
	if e.huddle(t).Ending || e.lastView(t).Ending {
		t.Fatalf("still ending: %+v", e.huddle(t))
	}
}

// endWithRecord runs a huddle with a conference to its end and returns the card post ID.
func endWithRecord(t *testing.T, e *env) string {
	t.Helper()
	e.start(t, "u1", "c1")
	e.live(gAnna)
	e.tick()
	postID := e.huddle(t).PostID
	e.advance(time.Minute)
	e.endConference()
	if e.followup(t) == nil {
		t.Fatal("no followup")
	}
	return postID
}

func botReplies(e *env) []*model.Post {
	var out []*model.Post
	for _, p := range e.mm.created {
		if p.UserId == "bot" {
			out = append(out, p)
		}
	}
	return out
}

func TestFollowupPostsArtifactOnce(t *testing.T) {
	e := newTickEnv(t)
	postID := endWithRecord(t, e)
	e.meet.ArtifactList = []google.Artifact{{Kind: "recording", Name: testRecord + "/recordings/1", URL: "https://drive.test/rec"}}

	e.advance(4 * time.Minute)
	e.tick()
	if e.countCalls("Artifacts") != 0 {
		t.Fatal("followup polled before NextAt")
	}

	e.advance(time.Minute)
	e.tick()
	replies := botReplies(e)
	if len(replies) != 1 || replies[0].RootId != postID || replies[0].ChannelId != "c1" ||
		!strings.Contains(replies[0].Message, "https://drive.test/rec") || !strings.Contains(replies[0].Message, "Aufzeichnung") {
		t.Fatalf("replies = %+v", replies)
	}
	if !slices.Contains(e.meet.Calls, "Artifacts u1@corp.test "+testRecord) {
		t.Fatalf("calls = %q", e.meet.Calls)
	}

	e.meet.ArtifactList = append(e.meet.ArtifactList, google.Artifact{Kind: "transcript", Name: testRecord + "/transcripts/1", URL: "https://docs.test/tr"})
	e.advance(5 * time.Minute)
	e.tick()
	e.advance(5 * time.Minute)
	e.tick()
	replies = botReplies(e)
	if len(replies) != 2 || !strings.Contains(replies[1].Message, "https://docs.test/tr") {
		t.Fatalf("replies = %+v", replies)
	}
}

func TestFollowupStopsAfterTwoHours(t *testing.T) {
	e := newTickEnv(t)
	endWithRecord(t, e)
	e.meet.ArtifactList = []google.Artifact{{Kind: "smart_notes", Name: "n1", URL: "https://docs.test/notes"}}

	e.advance(2*time.Hour + time.Second)
	e.tick()
	if e.followup(t) != nil || e.countCalls("Artifacts") != 0 || len(botReplies(e)) != 0 {
		t.Fatalf("followup = %+v calls = %q", e.followup(t), e.meet.Calls)
	}
}

// failSetKV fails Set/SetWithTTL for keys with prefix while fail is true.
type failSetKV struct {
	kv.KV
	prefix string
	fail   bool
}

func (f *failSetKV) Set(key string, v any) error {
	if f.fail && strings.HasPrefix(key, f.prefix) {
		return errors.New("kv down")
	}
	return f.KV.Set(key, v)
}

func (f *failSetKV) SetWithTTL(key string, v any, ttl time.Duration) error {
	if f.fail && strings.HasPrefix(key, f.prefix) {
		return errors.New("kv down")
	}
	return f.KV.SetWithTTL(key, v, ttl)
}

func TestFinishRetriesAfterRestrictFailure(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	e.meet.Err = map[string]error{"RestrictSpace": errors.New("boom")}
	e.advance(time.Minute)
	e.endConference()

	h := e.huddle(t) // still active, end decided
	if !h.Finishing || h.EndingSince.IsZero() || !slices.Contains(e.pres.calls, "left u1 "+id) || e.followup(t) != nil {
		t.Fatalf("huddle = %+v presence = %q followup = %+v", h, e.pres.calls, e.followup(t))
	}

	// The retry finishes even if the conference came back meanwhile.
	e.meet.Err = nil
	e.live(gAnna)
	e.advance(15 * time.Second)
	e.tick()
	e.assertEnded(t, id)
	if f := e.followup(t); f == nil || !slices.Equal(f.ConferenceRecords, []string{testRecord}) {
		t.Fatalf("followup = %+v", f)
	}
	if v := e.lastView(t); !v.Ended {
		t.Fatalf("view = %+v", v)
	}
}

func TestFinishRetriesAfterEndHuddleFailure(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	fkv := &failSetKV{KV: e.kv, prefix: "hid:"}
	e.s = e.service(NewStore(fkv))
	e.live(gAnna)
	e.tick()
	e.meet.ArtifactList = []google.Artifact{{Kind: "recording", Name: "rec1", URL: "https://drive.test/rec"}}
	fkv.fail = true

	e.advance(time.Minute)
	e.endConference() // followup saved, EndHuddle fails
	e.huddle(t)
	e.advance(5 * time.Minute)
	e.tick() // finish retried (fails again), followup posts rec1
	if f := e.followup(t); f == nil || !slices.Equal(f.Posted, []string{"rec1"}) {
		t.Fatalf("followup = %+v", f)
	}

	fkv.fail = false
	e.advance(15 * time.Second)
	e.tick()
	e.assertEnded(t, id)
	if f := e.followup(t); f == nil || !slices.Equal(f.Posted, []string{"rec1"}) {
		t.Fatalf("followup reset: %+v", f)
	}
	e.advance(5 * time.Minute)
	e.tick()
	if n := len(botReplies(e)); n != 1 {
		t.Fatalf("replies = %d, want 1", n)
	}
}

func TestJoinRejectedOnceEndDecided(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.meet.Err = map[string]error{"RestrictSpace": errors.New("boom")}
	e.advance(11 * time.Minute)
	e.tick() // nobody joined in 10 min; finish fails

	if _, err := e.s.Join(context.Background(), "u2", id); !errors.Is(err, ErrEnded) {
		t.Fatalf("join err = %v, want ErrEnded", err)
	}
}

func TestJoinAndInvitePollImmediately(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.advance(2 * time.Minute)
	e.tick() // NextPollAt = now + 15s

	e.advance(time.Second)
	if _, err := e.s.Join(context.Background(), "u2", id); err != nil {
		t.Fatal(err)
	}
	if h := e.huddle(t); !h.NextPollAt.Equal(e.clock) {
		t.Fatalf("join: next poll = %v, want %v", h.NextPollAt, e.clock)
	}

	e.tick()
	e.advance(time.Second)
	if err := e.s.Invite(context.Background(), "u1", id, []string{"u3"}); err != nil {
		t.Fatal(err)
	}
	if h := e.huddle(t); !h.NextPollAt.Equal(e.clock) {
		t.Fatalf("invite: next poll = %v, want %v", h.NextPollAt, e.clock)
	}
}

func TestFinishWithoutCardSkipsFollowup(t *testing.T) {
	e := newTickEnv(t)
	e.start(t, "u1", "c1")
	if _, err := e.store.UpdateHuddle("c1", func(h *Huddle) error { h.PostID = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	e.live(gAnna)
	e.tick()
	e.advance(time.Minute)
	e.endConference()
	if f := e.followup(t); f != nil {
		t.Fatalf("followup = %+v", f)
	}
}

func TestFollowupDroppedWhenRootDeleted(t *testing.T) {
	e := newTickEnv(t)
	endWithRecord(t, e)
	e.meet.ArtifactList = []google.Artifact{{Kind: "recording", Name: "rec1", URL: "https://drive.test/rec"}}
	e.mm.failCreate = func(*model.Post) *model.AppError {
		return model.NewAppError("fake", "invalid root", nil, "", http.StatusBadRequest)
	}
	e.advance(5 * time.Minute)
	e.tick()
	if f := e.followup(t); f != nil {
		t.Fatalf("followup = %+v", f)
	}
}

func TestFollowupRetriesOnlyFailedArtifacts(t *testing.T) {
	e := newTickEnv(t)
	endWithRecord(t, e)
	e.meet.ArtifactList = []google.Artifact{
		{Kind: "recording", Name: "rec1", URL: "https://drive.test/rec"},
		{Kind: "transcript", Name: "tr1", URL: "https://docs.test/tr"},
	}
	e.mm.failCreate = func(p *model.Post) *model.AppError {
		if strings.Contains(p.Message, "docs.test") {
			return model.NewAppError("fake", "down", nil, "", http.StatusInternalServerError)
		}
		return nil
	}
	e.advance(5 * time.Minute)
	e.tick()
	if f := e.followup(t); f == nil || !slices.Equal(f.Posted, []string{"rec1"}) {
		t.Fatalf("followup = %+v", f)
	}
	e.mm.failCreate = nil
	e.advance(5 * time.Minute)
	e.tick()
	if n := len(botReplies(e)); n != 2 {
		t.Fatalf("replies = %d, want 2", n)
	}
}

func TestFinishEndsAfterBoundDespitePermanentRestrictFailure(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	e.meet.Err = map[string]error{"RestrictSpace": errors.New("boom")}
	e.advance(time.Minute)
	e.endConference() // end decided, finish fails

	e.advance(finishBound - time.Second)
	e.tick()
	if h := e.huddle(t); !h.Finishing {
		t.Fatalf("ended before the bound: %+v", h)
	}
	e.advance(15 * time.Second) // next retry past the bound
	e.tick()
	e.assertEnded(t, id)
	if !slices.Contains(e.pres.calls, "left u1 "+id) || e.followup(t) == nil {
		t.Fatalf("presence = %q followup = %+v", e.pres.calls, e.followup(t))
	}
	if res := e.start(t, "u1", "c1"); res.AlreadyRunning || res.Huddle.ID == id {
		t.Fatalf("restart = %+v", res)
	}
}

// A missing key is no reason to leave the space OPEN: restricting is retried until finishBound.
func TestFinishRetriesRestrictWhileGoogleNotConfigured(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	e.meet.Err = map[string]error{"RestrictSpace": google.ErrNotConfigured} // key removed meanwhile
	e.advance(time.Minute)
	e.endConference()
	if h := e.huddle(t); !h.Finishing {
		t.Fatalf("huddle = %+v", h)
	}
	e.meet.Err = nil // key back
	e.advance(15 * time.Second)
	e.tick()
	e.assertEnded(t, id)
	if n := e.countCalls("RestrictSpace"); n != 2 {
		t.Fatalf("RestrictSpace calls = %d", n)
	}
}

// blockingMeet blocks the first GetSpace until its ctx is done.
type blockingMeet struct {
	*google.Fake
	blocked atomic.Bool
}

func (b *blockingMeet) GetSpace(ctx context.Context, owner, space string) (google.Space, error) {
	if b.blocked.CompareAndSwap(false, true) {
		<-ctx.Done()
		return google.Space{}, ctx.Err()
	}
	return b.Fake.GetSpace(ctx, owner, space)
}

func TestTickBlockedPollTimesOut(t *testing.T) {
	defer func(d time.Duration) { googleTimeout = d }(googleTimeout)
	googleTimeout = 50 * time.Millisecond
	e := newTickEnv(t)
	e.start(t, "u1", "c1")
	e.start(t, "u1", "dm")
	e.live(gAnna)
	e.s = newService(e.mm, &blockingMeet{Fake: e.meet}, e.store, func() Config { return e.cfg }, func() time.Time { return e.clock }, e.ring, e.pres)

	done := make(chan struct{})
	go func() { e.tick(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tick blocked")
	}
	if n := e.countCalls("ActiveParticipants"); n != 1 {
		t.Fatalf("other huddle not polled: %q", e.meet.Calls)
	}
}

func TestTickNameCorrelationIsNotCached(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	if _, err := e.s.Join(context.Background(), "u3", id); err != nil {
		t.Fatal(err)
	}
	gCleo := google.Participant{GoogleUserID: "users/77", DisplayName: "Cleo"} // outside the Directory
	e.live(gAnna, gCleo)
	e.tick()

	if h := e.huddle(t); h.Participants["u3"].State != StateConfirmed {
		t.Fatalf("participants = %+v", h.Participants)
	}
	if uid, _ := e.store.GoogleMapping("users/77"); uid != "" {
		t.Fatalf("name correlation cached: gid:users/77 = %q", uid)
	}
}

func TestEndWhileGoogleDownEndsAfterBound(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	down := &googleapi.Error{Code: http.StatusServiceUnavailable}
	e.meet.Err = map[string]error{"EndActiveConference": down, "GetSpace": down, "RestrictSpace": down}
	if err := e.s.End(context.Background(), "u1", id); err != nil {
		t.Fatal(err)
	}

	e.advance(finishBound)
	e.tick()
	if h := e.huddle(t); h.EndingSince.IsZero() || h.Finishing {
		t.Fatalf("ended before the bound: %+v", h)
	}
	e.advance(15 * time.Second) // next poll, past the bound
	e.tick()                    // end decided, finish gives up on RestrictSpace at once
	e.assertEnded(t, id)
	if !slices.Contains(e.pres.calls, "left u1 "+id) {
		t.Fatalf("presence = %q", e.pres.calls)
	}
}

func TestTickSpaceDeletedEndsHuddle(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	gone := &googleapi.Error{Code: http.StatusNotFound}
	e.meet.Err = map[string]error{"GetSpace": gone, "RestrictSpace": gone}
	e.advance(time.Minute)
	e.tick()
	e.advance(endGrace)
	e.tick()
	e.assertEnded(t, id)
	if !slices.Contains(e.pres.calls, "left u1 "+id) {
		t.Fatalf("presence = %q", e.pres.calls)
	}
}

// A missing or broken key is a Google outage, not the end of every huddle: the huddle keeps its
// state; a requested end still ends it after finishBound, the 12 h cap still applies.
func TestTickGoogleNotConfiguredKeepsHuddle(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	e.meet.Err = map[string]error{"GetSpace": google.ErrNotConfigured, "EndActiveConference": google.ErrNotConfigured, "RestrictSpace": google.ErrNotConfigured}
	for range 3 {
		e.advance(time.Hour)
		e.tick()
	}
	h := e.huddle(t)
	if h.Finishing || !h.EndingSince.IsZero() || !h.GoneSince.IsZero() || h.Participants["u1"].State != StateConfirmed || len(e.pres.calls) != 1 {
		t.Fatalf("huddle = %+v presence = %q", h, e.pres.calls)
	}

	if err := e.s.End(context.Background(), "u1", id); err != nil {
		t.Fatal(err)
	}
	e.advance(finishBound)
	e.tick()
	e.huddle(t) // within the bound
	e.advance(15 * time.Second)
	e.tick()
	e.assertEnded(t, id)

	e.meet.Err = nil
	id = e.start(t, "u1", "c1").Huddle.ID
	e.meet.Err = map[string]error{"GetSpace": google.ErrNotConfigured}
	e.advance(12*time.Hour + time.Second)
	e.tick()
	e.assertEnded(t, id)
}

// A reload or brief drop (conference gone < endGrace, then a new conference) keeps the huddle.
func TestTickConferenceGapKeepsHuddle(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	if _, err := e.s.Join(context.Background(), "u2", id); err != nil {
		t.Fatal(err)
	}
	e.live(gAnna, gBen)
	e.tick()
	pres, events := len(e.pres.calls), len(e.mm.ws)

	e.meet.Space.ActiveConference = ""
	e.advance(5 * time.Second)
	e.tick()
	h := e.huddle(t)
	if !h.GoneSince.Equal(e.clock) || !h.NextPollAt.Equal(e.clock.Add(pollFast)) || h.Finishing {
		t.Fatalf("huddle = %+v", h)
	}
	e.advance(endGrace - time.Second)
	e.tick()

	e.meet.Space.ActiveConference = "conferenceRecords/r2"
	e.advance(pollFast)
	e.tick()
	h = e.huddle(t)
	if h.ID != id || !slices.Equal(h.ConferenceRecords, []string{testRecord, "conferenceRecords/r2"}) || !h.GoneSince.IsZero() || h.Finishing ||
		h.Participants["u1"].State != StateConfirmed || h.Participants["u2"].State != StateConfirmed {
		t.Fatalf("huddle = %+v", h)
	}
	if len(e.pres.calls) != pres || len(e.mm.ws) != events {
		t.Fatalf("presence = %q ws = %+v", e.pres.calls[pres:], e.mm.ws[events:])
	}

	// Gone again: a fresh grace window, then the end.
	e.meet.Space.ActiveConference = ""
	e.advance(endGrace)
	e.tick()
	e.huddle(t)
	e.advance(endGrace)
	e.tick()
	e.assertEnded(t, id)
	if f := e.followup(t); f == nil || !slices.Equal(f.ConferenceRecords, []string{testRecord, "conferenceRecords/r2"}) {
		t.Fatalf("followup = %+v", f)
	}

	// Follow-ups ask every conference for artifacts; the same artifact is posted once.
	e.meet.ArtifactList = []google.Artifact{{Kind: "recording", Name: "rec1", URL: "https://drive.test/rec"}}
	e.advance(followupEvery)
	e.tick()
	for _, rec := range []string{testRecord, "conferenceRecords/r2"} {
		if !slices.Contains(e.meet.Calls, "Artifacts u1@corp.test "+rec) {
			t.Fatalf("%s not asked: %q", rec, e.meet.Calls)
		}
	}
	if n := len(botReplies(e)); n != 1 {
		t.Fatalf("replies = %d, want 1", n)
	}
}

// Fast polling only while waiting out endGrace, not while Google fails or the end is being finished.
func TestTickFastPollOnlyWithinGrace(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	e.advance(boostFor) // past the start boost
	e.meet.Space.ActiveConference = ""
	e.tick()
	if h := e.huddle(t); !h.NextPollAt.Equal(e.clock.Add(pollFast)) {
		t.Fatalf("grace: next poll = %v", h.NextPollAt)
	}
	e.meet.Err = map[string]error{"GetSpace": errors.New("boom"), "RestrictSpace": errors.New("boom")}
	e.advance(pollFast)
	e.tick()
	if h := e.huddle(t); !h.NextPollAt.Equal(e.clock.Add(pollSlow)) || h.GoneSince.IsZero() {
		t.Fatalf("google failing: huddle = %+v", h)
	}
	delete(e.meet.Err, "GetSpace")
	e.advance(endGrace)
	e.tick() // end decided, RestrictSpace fails
	if h := e.huddle(t); !h.Finishing || !h.NextPollAt.Equal(e.clock.Add(pollSlow)) {
		t.Fatalf("finishing: huddle = %+v", h)
	}
	e.meet.Err = nil
	e.advance(pollSlow)
	e.tick()
	e.assertEnded(t, id)
}

func TestDecideEnd(t *testing.T) {
	start := t0
	recs := []string{testRecord}
	for _, tc := range []struct {
		name string
		h    Huddle
		gone bool
		age  time.Duration
		want bool
		// wantGone: GoneSince afterwards (zero = unset)
		wantGone time.Time
	}{
		{name: "running", h: Huddle{ConferenceRecords: recs}, age: time.Hour},
		{name: "finishing", h: Huddle{Finishing: true}, age: time.Minute, want: true},
		{name: "12 h cap", h: Huddle{ConferenceRecords: recs}, age: maxHuddle + time.Second, want: true},
		{name: "no conference yet", gone: true, age: noConference},
		{name: "no conference for 10 min", gone: true, age: noConference + time.Second, want: true},
		{name: "end requested, conference gone: no grace", h: Huddle{ConferenceRecords: recs, EndingSince: start.Add(time.Minute)}, gone: true, age: 2 * time.Minute, want: true},
		{name: "end requested, google failing", h: Huddle{ConferenceRecords: recs, EndingSince: start}, age: finishBound},
		{name: "end requested, google failing past bound", h: Huddle{ConferenceRecords: recs, EndingSince: start}, age: finishBound + time.Second, want: true},
		{name: "conference just gone", h: Huddle{ConferenceRecords: recs}, gone: true, age: time.Hour, wantGone: start.Add(time.Hour)},
		{name: "gone within grace", h: Huddle{ConferenceRecords: recs, GoneSince: start.Add(time.Hour)}, gone: true, age: time.Hour + endGrace - time.Second, wantGone: start.Add(time.Hour)},
		{name: "gone for grace", h: Huddle{ConferenceRecords: recs, GoneSince: start.Add(time.Hour)}, gone: true, age: time.Hour + endGrace, want: true, wantGone: start.Add(time.Hour)},
		{name: "google failing while gone", h: Huddle{ConferenceRecords: recs, GoneSince: start.Add(time.Hour)}, age: 2 * time.Hour, wantGone: start.Add(time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := tc.h
			h.StartedAt = start
			if got := decideEnd(&h, tc.gone, start.Add(tc.age)); got != tc.want || !h.GoneSince.Equal(tc.wantGone) {
				t.Fatalf("decideEnd = %v GoneSince = %v", got, h.GoneSince)
			}
		})
	}
}

// countKV counts ListKeys calls.
type countKV struct {
	kv.KV
	lists atomic.Int32
}

func (c *countKV) ListKeys(prefix string) ([]string, error) {
	c.lists.Add(1)
	return c.KV.ListKeys(prefix)
}

func TestTickScansKeysOnce(t *testing.T) {
	e := newTickEnv(t)
	e.start(t, "u1", "c1")
	if err := e.store.SaveRing(&Ring{ID: "r1", To: "u2", State: ringRinging, CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SaveFollowup(&Followup{HuddleID: "old", Until: t0.Add(time.Hour), NextAt: t0.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	c := &countKV{KV: e.kv}
	e.s = e.service(NewStore(c))
	e.tick()
	if n := c.lists.Load(); n != 1 {
		t.Fatalf("ListKeys calls = %d, want 1", n)
	}
	if n := e.countCalls("GetSpace"); n != 1 {
		t.Fatalf("huddle not polled: %q", e.meet.Calls)
	}
	if !slices.Contains(e.ring.calls, "expire "+t0.Format(time.RFC3339)+" 1") {
		t.Fatalf("rings not expired: %q", e.ring.calls)
	}
}

func TestEndRetriedEachPollThenNormalEnd(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	e.meet.Err = map[string]error{"EndActiveConference": errors.New("boom")}
	if err := e.s.End(context.Background(), "u1", id); err != nil {
		t.Fatal(err)
	}
	base := e.countCalls("EndActiveConference")
	e.advance(15 * time.Second)
	e.tick()
	e.advance(15 * time.Second)
	e.tick()
	if n := e.countCalls("EndActiveConference") - base; n != 2 {
		t.Fatalf("retries = %d", n)
	}
	if h := e.huddle(t); h.Finishing {
		t.Fatal("ended early")
	}
	// The retry finally works: the conference is gone, normal end.
	delete(e.meet.Err, "EndActiveConference")
	e.meet.Space.ActiveConference = ""
	e.advance(15 * time.Second)
	e.tick()
	e.assertEnded(t, id)
}

func TestEndEndsAfterBoundWhileConferenceStaysActive(t *testing.T) {
	e := newTickEnv(t)
	id := e.start(t, "u1", "c1").Huddle.ID
	e.live(gAnna)
	e.tick()
	e.meet.Err = map[string]error{"EndActiveConference": errors.New("boom")}
	if err := e.s.End(context.Background(), "u1", id); err != nil {
		t.Fatal(err)
	}
	e.advance(finishBound + 15*time.Second)
	e.tick()
	e.assertEnded(t, id)
}
