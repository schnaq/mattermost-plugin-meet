package huddle

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/schnaq/mattermost-plugin-meet/server/kv"
)

// interferingKV injects a foreign write before the first CompareAndSet.
type interferingKV struct {
	kv.KV
	once   bool
	inject func()
}

func (i *interferingKV) CompareAndSet(key string, old, new any) (bool, error) {
	if !i.once {
		i.once = true
		i.inject()
	}
	return i.KV.CompareAndSet(key, old, new)
}

func TestUpdateHuddleRetriesOnConflict(t *testing.T) {
	base := kv.NewMem()
	s := NewStore(base)
	if ok, err := s.CreateHuddle(&Huddle{ID: "h1", ChannelID: "c1"}); err != nil || !ok {
		t.Fatalf("create: %v %v", ok, err)
	}
	if ok, _ := s.CreateHuddle(&Huddle{ID: "h2", ChannelID: "c1"}); ok {
		t.Fatal("second create for same channel must not succeed")
	}
	w := NewStore(&interferingKV{KV: base, inject: func() {
		_, _ = NewStore(base).UpdateHuddle("c1", func(h *Huddle) error { h.PostID = "p"; return nil })
	}})
	attempts := 0
	got, err := w.UpdateHuddle("c1", func(h *Huddle) error { attempts++; h.Space = "s"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || got.PostID != "p" || got.Space != "s" {
		t.Fatalf("attempts=%d got=%+v", attempts, got)
	}
}

func TestUpdateHuddleGivesUp(t *testing.T) {
	base := kv.NewMem()
	s := NewStore(base)
	_, _ = s.CreateHuddle(&Huddle{ID: "h1", ChannelID: "c1"})
	n := 0
	_, err := s.UpdateHuddle("c1", func(h *Huddle) error {
		n++
		return base.Set("h:c1", &Huddle{ID: "h1", ChannelID: "c1", PostID: strings.Repeat("a", n)})
	})
	if err == nil || n != 5 {
		t.Fatalf("err=%v attempts=%d", err, n)
	}
}

func TestEndHuddleKeepsIDLookup(t *testing.T) {
	s := NewStore(kv.NewMem())
	h := &Huddle{ID: "h1", ChannelID: "c1"}
	_, _ = s.CreateHuddle(h)
	if got, err := s.HuddleByID("h1"); got == nil || err != nil {
		t.Fatalf("active: %v %v", got, err)
	}
	if err := s.EndHuddle(h); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Huddle("c1"); got != nil {
		t.Fatal("huddle should be gone")
	}
	if _, err := s.HuddleByID("h1"); !errors.Is(err, ErrEnded) {
		t.Fatalf("err=%v", err)
	}
	if _, err := s.HuddleByID("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: err=%v", err)
	}
	if hs, _, _ := s.ActiveHuddles(); len(hs) != 0 {
		t.Fatal("no active huddles expected")
	}
}

func TestTakeRateLimit(t *testing.T) {
	now := time.Now()
	s := NewStore(kv.NewMemClock(func() time.Time { return now }))
	if !s.TakeRateLimit("a", "b") {
		t.Fatal("first call must pass")
	}
	now = now.Add(30 * time.Second)
	if s.TakeRateLimit("a", "b") {
		t.Fatal("within 60s must be limited")
	}
	now = now.Add(31 * time.Second)
	if !s.TakeRateLimit("a", "b") {
		t.Fatal("after 61s must pass")
	}
}

func TestRingFollowupStatusMapping(t *testing.T) {
	s := NewStore(kv.NewMem())
	_ = s.SaveRing(&Ring{ID: "r1", State: "ringing"})
	_ = s.SaveRing(&Ring{ID: "r2", State: "accepted"})
	if r, _ := s.Ring("r1"); r == nil || r.ID != "r1" {
		t.Fatal("ring lookup")
	}
	if rs, _ := s.Rings(); len(rs) != 2 {
		t.Fatalf("rings=%d", len(rs))
	}
	_ = s.SaveFollowup(&Followup{HuddleID: "h1"})
	if _, _, f, _ := s.TickState(); len(f) != 1 {
		t.Fatal("followups")
	}
	_ = s.DeleteFollowup("h1")
	if _, _, f, _ := s.TickState(); len(f) != 0 {
		t.Fatal("followup delete")
	}
	type cs struct {
		Emoji, Text string
		Duration    string
	}
	_ = s.SaveStatus("u1", cs{Emoji: "tada", Text: "busy", Duration: "today"})
	var got cs
	if ok, _ := s.SavedStatus("u1", &got); !ok || got.Text != "busy" || got.Emoji != "tada" {
		t.Fatalf("status %+v", got)
	}
	_ = s.SaveStatus("u2", (*cs)(nil)) // "no status" is storable as JSON null
	if ok, _ := s.SavedStatus("u2", &got); !ok {
		t.Fatal("null status must be found")
	}
	_ = s.DeleteStatus("u1")
	if ok, _ := s.SavedStatus("u1", &got); ok {
		t.Fatal("status delete")
	}
	_ = s.SaveGoogleMapping("g1", "u1")
	if u, _ := s.GoogleMapping("g1"); u != "u1" {
		t.Fatal("mapping")
	}
}

func TestGoogleCacheTTLAndClear(t *testing.T) {
	now := time.Now()
	s := NewStore(kv.NewMemClock(func() time.Time { return now }))
	_ = s.SaveGoogleMapping("g1", "u1")
	_ = s.SaveGoogleMiss("g2")
	_ = s.SaveRing(&Ring{ID: "r1"})
	if err := s.ClearGoogleCache(); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.GoogleMapping("g1"); u != "" {
		t.Fatalf("gid: not cleared: %q", u)
	}
	if missed, _ := s.GoogleMiss("g2"); missed {
		t.Fatal("gx: not cleared")
	}
	if r, _ := s.Ring("r1"); r == nil {
		t.Fatal("other keys must stay")
	}

	_ = s.SaveGoogleMapping("g1", "u1")
	now = now.Add(mappingTTL - time.Second)
	if u, _ := s.GoogleMapping("g1"); u != "u1" {
		t.Fatal("mapping expired early")
	}
	now = now.Add(2 * time.Second)
	if u, _ := s.GoogleMapping("g1"); u != "" {
		t.Fatalf("mapping not expired: %q", u)
	}
}

// One unreadable entry must not hide the others from the tick or the huddle list.
func TestLoadSkipsBadEntry(t *testing.T) {
	e := newEnv(t)
	e.start(t, "u1", "c1")
	if err := e.kv.Set("h:bad", "not a huddle"); err != nil {
		t.Fatal(err)
	}
	hs, _, _, err := e.store.TickState()
	if len(hs) != 1 || hs[0].ChannelID != "c1" || err == nil || !strings.Contains(err.Error(), "h:bad") {
		t.Fatalf("TickState = %+v %v", hs, err)
	}
	// The tick logs the bad entry once, not every 2 s.
	logs := len(e.mm.logs)
	for range 3 {
		e.tick()
	}
	if n := len(e.mm.logs) - logs; n != 1 {
		t.Fatalf("tick logs = %q", e.mm.logs[logs:])
	}
	logs = len(e.mm.logs)
	if views, err := e.s.Active("u1"); err != nil || len(views) != 1 || len(e.mm.logs) != logs+1 {
		t.Fatalf("Active = %+v %v", views, err)
	}
	if err := e.kv.Delete(pHuddle + "c1"); err != nil {
		t.Fatal(err)
	}
	if views, err := e.s.Active("u1"); err != nil || len(views) != 0 {
		t.Fatalf("only the bad entry: Active = %#v %v", views, err)
	}
}
