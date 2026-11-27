package huddle

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/schnaq/mattermost-plugin-meet/server/kv"
)

type ringEnv struct {
	*env
	rs ringer
}

func newRingEnv(t *testing.T) *ringEnv {
	t.Helper()
	e := newEnv(t)
	e.cfg.BotUserID = "bot"
	e.mm.addUser("bot", "Bot", "en")
	e.mm.addChannel("gm", model.ChannelTypeGroup, "u1", "u2", "u3")
	rs := newRingService(e.mm, e.store, func() Config { return e.cfg }, func() time.Time { return e.clock })
	e.s.ringer = rs
	return &ringEnv{e, rs}
}

func (e *ringEnv) huddle(t *testing.T, channel string) *Huddle {
	t.Helper()
	post, _ := e.mm.CreatePost(&model.Post{UserId: "u1", ChannelId: channel})
	h := &Huddle{ID: "h1", ChannelID: channel, StarterUserID: "u1", PostID: post.Id, StartedAt: e.clock, Participants: map[string]Participant{}}
	if ok, err := e.store.CreateHuddle(h); !ok || err != nil {
		t.Fatal(ok, err)
	}
	return h
}

func (e *ringEnv) events(name string) []wsEvent {
	var out []wsEvent
	for _, w := range e.mm.ws {
		if w.Event == name {
			out = append(out, w)
		}
	}
	return out
}

// assertOutcome checks the single ring_outcome event: caller u1 only, callee u2, state.
func assertOutcome(t *testing.T, e *ringEnv, state string) {
	t.Helper()
	ws := e.events("ring_outcome")
	if len(ws) != 1 || ws[0].Broadcast.UserId != "u1" || ws[0].Payload["state"] != state || ws[0].Payload["to"] != "u2" || ws[0].Payload["ring_id"] == nil {
		t.Errorf("ring_outcome %+v", ws)
	}
}

// expire runs the ring expiry over the stored rings, as the tick does.
func (e *ringEnv) expire(now time.Time) {
	rs, _ := e.store.Rings()
	e.rs.expire(context.Background(), now, rs)
}

func (e *ringEnv) onlyRing(t *testing.T) *Ring {
	t.Helper()
	rs, _ := e.store.Rings()
	if len(rs) != 1 {
		t.Fatalf("rings = %d", len(rs))
	}
	return rs[0]
}

func TestRingChannelRingsAllOtherMembers(t *testing.T) {
	e := newRingEnv(t)
	h := e.huddle(t, "gm")
	if err := e.rs.ringChannel(context.Background(), h, "u1"); err != nil {
		t.Fatal(err)
	}
	rings, _ := e.store.Rings()
	if len(rings) != 2 {
		t.Fatalf("rings = %d", len(rings))
	}
	for _, r := range rings {
		if r.Targeted || r.PostID != h.PostID || r.From != "u1" || r.HuddleID != "h1" || r.To == "u1" {
			t.Errorf("ring %+v", r)
		}
	}
	ws := e.events("ring")
	if len(ws) != 2 {
		t.Fatalf("ws = %d", len(ws))
	}
	p := ws[0].Payload
	if p["huddle_id"] != "h1" || p["channel_id"] != "gm" || p["from"] != "u1" || p["ring_id"] == "" || ws[0].Broadcast.UserId == "u1" {
		t.Errorf("payload %v %v", p, ws[0].Broadcast)
	}
}

func TestRingSkipsDNDButRecordsRing(t *testing.T) {
	e := newRingEnv(t)
	e.mm.statuses["u2"] = &model.Status{UserId: "u2", Status: model.StatusDnd}
	e.mm.statuses["u3"] = &model.Status{UserId: "u3", Status: model.StatusOutOfOffice}
	if err := e.rs.ringChannel(context.Background(), e.huddle(t, "gm"), "u1"); err != nil {
		t.Fatal(err)
	}
	if rings, _ := e.store.Rings(); len(rings) != 2 {
		t.Errorf("rings = %d", len(rings))
	}
	if n := len(e.events("ring")); n != 0 {
		t.Errorf("ws rings = %d", n)
	}
}

func TestRingUsersSendsBotDM(t *testing.T) {
	e := newRingEnv(t)
	e.mm.channels["c1"].Name, e.mm.channels["c1"].DisplayName = "town-square", "Town Square"
	h := e.huddle(t, "c1")
	if err := e.rs.ringUsers(context.Background(), h, "u1", []string{"u2", "u3"}); err != nil {
		t.Fatal(err)
	}
	var dms []*model.Post
	for _, p := range e.mm.created {
		if p.UserId == "bot" {
			dms = append(dms, p)
		}
	}
	if len(dms) != 2 {
		t.Fatalf("bot posts = %d", len(dms))
	}
	var dm *model.Post
	for _, p := range dms {
		if p.ChannelId == model.GetDMNameFromIds("bot", "u2") {
			dm = p
		}
	}
	if dm == nil || dm.Message != "@u1 invites you to the huddle in **Town Square**" {
		t.Fatalf("dm %+v", dm)
	}
	if strings.Contains(dm.Message, "meet.google") || strings.Contains(dm.Message, testMeetURI) {
		t.Error("meet url leaked")
	}
	act := dm.Attachments()[0].Actions[0]
	if act.Integration.URL != e.cfg.PluginURL+"/api/v1/actions/join" || act.Integration.Context["huddle_id"] != "h1" {
		t.Errorf("action %+v", act.Integration)
	}
	if dm.GetProp("huddle_id") != "h1" {
		t.Errorf("huddle_id prop = %v", dm.GetProp("huddle_id"))
	}
	for _, r := range func() []*Ring { rs, _ := e.store.Rings(); return rs }() {
		if !r.Targeted || r.PostID == "" || r.To != "u2" && r.To != "u3" {
			t.Errorf("ring %+v", r)
		}
	}
	if n := len(e.events("ring")); n != 2 {
		t.Errorf("ws = %d", n)
	}
	// German recipient
	e.mm.created = nil
	_ = e.rs.ringUsers(context.Background(), h, "u2", []string{"u1"})
	if got := e.mm.created[0].Message; got != "@u2 lädt dich ins Huddle in **Town Square** ein" {
		t.Errorf("de = %q", got)
	}
}

func ringOne(t *testing.T, e *ringEnv) *Ring {
	t.Helper()
	e.mm.addChannel("d", model.ChannelTypeDirect, "u1", "u2")
	h := e.huddle(t, "d")
	if err := e.rs.ringChannel(context.Background(), h, "u1"); err != nil {
		t.Fatal(err)
	}
	e.mm.ws = nil
	return e.onlyRing(t)
}

func TestRespondAcceptEndsRingEverywhere(t *testing.T) {
	e := newRingEnv(t)
	r := ringOne(t, e)
	if _, err := e.s.RespondRing(context.Background(), "u2", r.ID, "accept", ""); err != nil {
		t.Fatal(err)
	}
	if got := e.onlyRing(t).State; got != "accepted" {
		t.Errorf("state %s", got)
	}
	ws := e.events("ring_ended")
	if len(ws) != 2 || ws[0].Payload["state"] != "accepted" || ws[0].Payload["ring_id"] != r.ID {
		t.Fatalf("ws %+v", ws)
	}
	if ws[0].Broadcast.UserId != "u2" || ws[1].Broadcast.UserId != "u1" {
		t.Errorf("targets %v %v", ws[0].Broadcast, ws[1].Broadcast)
	}
	if _, err := e.s.RespondRing(context.Background(), "u2", r.ID, "decline", ""); err != ErrEnded {
		t.Errorf("second respond = %v", err)
	}
}

func TestRespondOnlyByTarget(t *testing.T) {
	e := newRingEnv(t)
	r := ringOne(t, e)
	if _, err := e.s.RespondRing(context.Background(), "u1", r.ID, "accept", ""); err != ErrForbidden {
		t.Errorf("err = %v", err)
	}
	if _, err := e.s.RespondRing(context.Background(), "u2", "nope", "accept", ""); err != ErrNotFound {
		t.Errorf("err = %v", err)
	}
	if _, err := e.s.RespondRing(context.Background(), "u2", r.ID, "dance", ""); !errors.Is(err, ErrBadRequest) {
		t.Errorf("err = %v", err)
	}
	if e.onlyRing(t).State != "ringing" || len(e.events("ring_ended")) != 0 {
		t.Error("state changed")
	}
}

func TestQuickReplyPostsAsCallee(t *testing.T) {
	e := newRingEnv(t)
	r := ringOne(t, e)
	ctx := context.Background()
	if _, err := e.s.RespondRing(ctx, "u2", r.ID, "quickreply", "   "); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("empty = %v", err)
	}
	if e.onlyRing(t).State != "ringing" {
		t.Fatal("state changed on empty reply")
	}
	if _, err := e.s.RespondRing(ctx, "u2", r.ID, "quickreply", " "+strings.Repeat("ä", 300)); err != nil {
		t.Fatal(err)
	}
	last := e.mm.created[len(e.mm.created)-1]
	if last.UserId != "u2" || last.ChannelId != model.GetDMNameFromIds("u2", "u1") || len([]rune(last.Message)) != 200 {
		t.Errorf("post %+v", last)
	}
	if e.onlyRing(t).State != "declined" {
		t.Error("not declined")
	}
	if ws := e.events("ring_ended"); len(ws) != 2 || ws[0].Payload["state"] != "declined" {
		t.Errorf("ring_ended: %+v", ws)
	}
	if ws := e.events("ring_outcome"); len(ws) != 0 {
		t.Errorf("a quick reply has no outcome: %+v", ws)
	}
}

func TestRingUsersInGroupOmitsChannel(t *testing.T) {
	e := newRingEnv(t)
	e.mm.users["u2"].Locale = "de"
	h := e.huddle(t, "gm")
	if err := e.rs.ringUsers(context.Background(), h, "u1", []string{"u2", "u3"}); err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, p := range e.mm.created {
		if p.UserId == "bot" {
			msgs = append(msgs, p.Message)
		}
	}
	assertStrings(t, "invites", msgs, []string{"@u1 lädt dich zu einem Huddle ein", "@u1 invites you to a huddle"})
}

func TestExpireTargetedUpdatesBotDM(t *testing.T) {
	e := newRingEnv(t)
	e.mm.users["u1"].Username = "anna"
	h := e.huddle(t, "c1")
	_ = e.rs.ringUsers(context.Background(), h, "u1", []string{"u2"})
	e.mm.ws = nil
	e.expire(e.clock.Add(20 * time.Second))
	if len(e.mm.updated) != 0 {
		t.Fatal("expired too early")
	}
	e.expire(e.clock.Add(31 * time.Second))
	if len(e.mm.updated) != 1 || e.mm.updated[0].Message != "Missed huddle call from anna" || len(e.mm.updated[0].Attachments()) != 0 {
		t.Fatalf("updated %+v", e.mm.updated)
	}
	if ws := e.events("ring_ended"); len(ws) != 2 || ws[0].Payload["state"] != "missed" {
		t.Errorf("ws %+v", ws)
	}
	assertOutcome(t, e, "missed")
	if rs, _ := e.store.Rings(); len(rs) != 0 {
		t.Error("ring not deleted")
	}
}

func TestExpireUntargetedPostsMissedOnceUnlessAccepted(t *testing.T) {
	ctx := context.Background()
	t.Run("once", func(t *testing.T) {
		e := newRingEnv(t)
		h := e.huddle(t, "gm")
		_ = e.rs.ringChannel(ctx, h, "u1")
		n := len(e.mm.created)
		e.expire(e.clock.Add(31 * time.Second))
		e.expire(e.clock.Add(40 * time.Second))
		if len(e.mm.created) != n+1 {
			t.Fatalf("posts = %d", len(e.mm.created)-n)
		}
		if p := e.mm.created[n]; p.RootId != h.PostID || p.Message != "Verpasster Huddle-Anruf" || p.UserId != "u1" {
			t.Errorf("post %+v", p)
		}
		if rs, _ := e.store.Rings(); len(rs) != 0 {
			t.Error("rings left")
		}
	})
	t.Run("accepted", func(t *testing.T) {
		e := newRingEnv(t)
		h := e.huddle(t, "gm")
		_ = e.rs.ringChannel(ctx, h, "u1")
		rs, _ := e.store.Rings()
		if _, err := e.s.RespondRing(ctx, rs[0].To, rs[0].ID, "accept", ""); err != nil {
			t.Fatal(err)
		}
		n := len(e.mm.created)
		e.expire(e.clock.Add(31 * time.Second))
		if len(e.mm.created) != n {
			t.Error("missed hint despite accepted ring")
		}
		if rs, _ := e.store.Rings(); len(rs) != 0 {
			t.Error("rings left")
		}
	})
}

func TestJoinSettlesOpenRing(t *testing.T) {
	e := newRingEnv(t)
	ctx := context.Background()
	e.mm.channels["c1"].Name = "town"
	res, err := e.s.Start(ctx, "u1", "c1")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := e.store.Huddle("c1")
	if err = e.rs.ringUsers(ctx, h, "u1", []string{"u2"}); err != nil {
		t.Fatal(err)
	}
	e.mm.ws = nil
	if _, err = e.s.Join(ctx, "u2", res.Huddle.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.onlyRing(t).State; got != "accepted" {
		t.Fatalf("state %s", got)
	}
	if ws := e.events("ring_ended"); len(ws) != 2 || ws[0].Payload["state"] != "accepted" || ws[0].Broadcast.UserId != "u2" || ws[1].Broadcast.UserId != "u1" {
		t.Errorf("ws %+v", ws)
	}
	n, u := len(e.mm.created), len(e.mm.updated) // the join already refreshed the card
	e.expire(e.clock.Add(31 * time.Second))
	if len(e.mm.updated) != u || len(e.mm.created) != n {
		t.Errorf("missed DM/hint after join: %d updated, %d created", len(e.mm.updated)-u, len(e.mm.created)-n)
	}
}

func TestMissSkippedWhenTargetParticipant(t *testing.T) {
	ctx := context.Background()
	for _, targeted := range []bool{true, false} {
		e := newRingEnv(t)
		h := e.huddle(t, "gm")
		if targeted {
			_ = e.rs.ringUsers(ctx, h, "u1", []string{"u2"})
		} else {
			_ = e.rs.ringChannel(ctx, h, "u1")
		}
		// u2 is (or was) in the huddle; the ring itself was never settled.
		_, _ = e.store.UpdateHuddle("gm", func(x *Huddle) error { x.SeenUserIDs = []string{"u2"}; return nil })
		n := len(e.mm.created)
		e.expire(e.clock.Add(31 * time.Second))
		if len(e.mm.updated) != 0 || len(e.mm.created) != n {
			t.Errorf("targeted=%v: missed DM/hint posted", targeted)
		}
	}
}

func TestExpireSkipsRingAnsweredAfterSnapshot(t *testing.T) {
	e := newRingEnv(t)
	h := e.huddle(t, "c1")
	_ = e.rs.ringUsers(context.Background(), h, "u1", []string{"u2"})
	snapshot, _ := e.store.Rings()
	r := e.onlyRing(t)
	if _, err := e.s.RespondRing(context.Background(), "u2", r.ID, "decline", ""); err != nil {
		t.Fatal(err)
	}
	e.mm.ws = nil
	e.rs.expire(context.Background(), e.clock.Add(31*time.Second), snapshot)
	if len(e.mm.updated) != 0 || len(e.events("ring_ended")) != 0 {
		t.Fatalf("answered ring treated as missed: %+v", e.mm.updated)
	}
}

type failGetKV struct{ kv.KV }

func (failGetKV) Get(string, any) (bool, error) { return false, errors.New("kv down") }

func TestExpireKeepsRingWhenRereadFails(t *testing.T) {
	e := newRingEnv(t)
	ringOne(t, e)
	rs, _ := e.store.Rings()
	e.mm.ws = nil
	rsv := newRingService(e.mm, NewStore(failGetKV{e.kv}), func() Config { return e.cfg }, func() time.Time { return e.clock })
	rsv.expire(context.Background(), e.clock.Add(31*time.Second), rs)
	if got, _ := e.store.Rings(); len(got) != 1 || len(e.events("ring_ended")) != 0 {
		t.Fatalf("ring dropped without missed notice: %+v", got)
	}
	e.expire(e.clock.Add(31 * time.Second)) // store back: next tick handles it
	if ws := e.events("ring_ended"); len(ws) != 2 || ws[0].Payload["state"] != "missed" {
		t.Fatalf("ws %+v", ws)
	}
}

func TestRingOutcomeOnlyForDeclineAndMissed(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ action, reply, want string }{
		{"decline", "", "declined"},
		{"accept", "", ""},
		{"quickreply", "later", ""},
	} {
		t.Run(tc.action, func(t *testing.T) {
			e := newRingEnv(t)
			r := ringOne(t, e)
			if _, err := e.s.RespondRing(ctx, "u2", r.ID, tc.action, tc.reply); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if ws := e.events("ring_outcome"); len(ws) != 0 {
					t.Errorf("ring_outcome %+v", ws)
				}
				return
			}
			assertOutcome(t, e, tc.want)
		})
	}
	t.Run("missed", func(t *testing.T) {
		e := newRingEnv(t)
		ringOne(t, e)
		e.expire(e.clock.Add(31 * time.Second))
		assertOutcome(t, e, "missed")
	})
}

func TestQuickReplyPostFailureKeepsRinging(t *testing.T) {
	e := newRingEnv(t)
	r := ringOne(t, e)
	ctx := context.Background()
	e.mm.failCreate = func(*model.Post) *model.AppError {
		return model.NewAppError("fake", "down", nil, "", http.StatusInternalServerError)
	}
	if _, err := e.s.RespondRing(ctx, "u2", r.ID, "quickreply", "later"); err == nil {
		t.Fatal("expected the post error")
	}
	if e.onlyRing(t).State != ringRinging || len(e.events("ring_ended")) != 0 {
		t.Fatalf("ring = %+v ws = %+v", e.onlyRing(t), e.mm.ws)
	}
	e.mm.failCreate = nil
	posts := len(e.mm.created)
	if _, err := e.s.RespondRing(ctx, "u2", r.ID, "quickreply", "later"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(e.mm.created) != posts+1 || e.mm.created[posts].Message != "later" || e.onlyRing(t).State != ringDeclined || len(e.events("ring_ended")) != 2 {
		t.Fatalf("posts = %+v ring = %+v", e.mm.created[posts:], e.onlyRing(t))
	}
}

func TestAcceptRefusedOnceHuddleEnds(t *testing.T) {
	for name, end := range map[string]func(*ringEnv, *Huddle){
		"end requested": func(e *ringEnv, h *Huddle) {
			if _, err := e.store.UpdateHuddle(h.ChannelID, func(x *Huddle) error { x.EndingSince = e.clock; return nil }); err != nil {
				t.Fatal(err)
			}
		},
		"ended": func(e *ringEnv, h *Huddle) {
			if err := e.store.EndHuddle(h); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newRingEnv(t)
			r := ringOne(t, e)
			h, _ := e.store.Huddle("d")
			end(e, h)
			if _, err := e.s.RespondRing(context.Background(), "u2", r.ID, "accept", ""); !errors.Is(err, ErrEnded) {
				t.Fatalf("accept = %v", err)
			}
			if e.onlyRing(t).State != ringRinging || len(e.events("ring_ended")) != 0 {
				t.Fatalf("ring = %+v ws = %+v", e.onlyRing(t), e.mm.ws)
			}
		})
	}
}

// The end closes open ring dialogs at once; the rings still become missed calls (bot DM update,
// caller toast) when they expire, and a late accept answers "ended".
func TestFinishClosesOpenRings(t *testing.T) {
	e := newRingEnv(t)
	e.mm.users["u1"].Username = "anna"
	h := e.huddle(t, "c1")
	if err := e.rs.ringUsers(context.Background(), h, "u1", []string{"u2"}); err != nil {
		t.Fatal(err)
	}
	r := e.onlyRing(t)
	// Another huddle's ring, younger: neither closed by this end nor expired below.
	other := &Ring{ID: "r-other", HuddleID: "h-other", To: "u3", From: "u1", State: ringRinging, CreatedAt: e.clock.Add(ringTimeout)}
	if err := e.store.SaveRing(other); err != nil {
		t.Fatal(err)
	}
	e.mm.ws = nil
	h.EndingSince = e.clock
	if err := e.s.finish(context.Background(), h, e.clock); err != nil {
		t.Fatal(err)
	}
	ws := e.events("ring_ended")
	if len(ws) != 2 || ws[0].Payload["ring_id"] != r.ID || ws[0].Broadcast.UserId != "u2" {
		t.Fatalf("ring_ended = %+v", ws)
	}
	if _, err := e.s.RespondRing(context.Background(), "u2", r.ID, "accept", ""); !errors.Is(err, ErrEnded) {
		t.Fatalf("late accept = %v", err)
	}

	e.mm.ws, e.mm.updated = nil, nil
	rings, _ := e.store.Rings()
	e.rs.expire(context.Background(), e.clock.Add(ringTimeout+time.Second), rings)
	if len(e.mm.updated) != 1 || e.mm.updated[0].Message != "Missed huddle call from anna" || len(e.mm.updated[0].Attachments()) != 0 {
		t.Fatalf("missed DM: %+v", e.mm.updated)
	}
	assertOutcome(t, e, "missed")
	if got, _ := e.store.Ring(other.ID); got == nil || got.State != ringRinging {
		t.Fatalf("other huddle's ring = %+v", got)
	}
}
