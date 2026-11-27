package huddle

import (
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/schnaq/mattermost-plugin-meet/server/kv"
)

func statusFixture(t *testing.T) (*fakeMM, Store, presence) {
	t.Helper()
	mm := newFakeMM()
	mm.addUser("u1", "Ann", "en")
	s := NewStore(kv.NewMem())
	cfg := func() Config { return Config{StatusEmoji: "headphones", StatusText: "In a huddle"} }
	return mm, s, newStatusPresence(mm, s, cfg)
}

// setUserStatus mirrors the user's own custom status on the fake user and the fake status table.
func setUserStatus(t *testing.T, mm *fakeMM, id string, cs *model.CustomStatus) {
	t.Helper()
	if err := mm.users[id].SetCustomStatus(cs); err != nil {
		t.Fatal(err)
	}
}

// syncUser makes GetUser reflect what UpdateUserCustomStatus stored (the real server does this).
func syncUser(t *testing.T, mm *fakeMM, id string) {
	t.Helper()
	if cs := mm.customStatus[id]; cs != nil {
		setUserStatus(t, mm, id, cs)
	}
}

func TestStatusSetAndRestored(t *testing.T) {
	mm, s, p := statusFixture(t)
	prev := &model.CustomStatus{Emoji: "palm_tree", Text: "Vacation"}
	setUserStatus(t, mm, "u1", prev)
	h := &Huddle{ID: "h1", ChannelID: "c1"}

	p.joined("u1", h)
	if got := mm.customStatus["u1"]; got == nil || got.Emoji != "headphones" || got.Text != "In a huddle" || got.Duration != "" {
		t.Fatalf("status not set: %+v", got)
	}
	syncUser(t, mm, "u1")
	p.left("u1", h)
	if got := mm.customStatus["u1"]; got == nil || got.Emoji != "palm_tree" || got.Text != "Vacation" {
		t.Fatalf("not restored: %+v", got)
	}
	if found, _ := s.SavedStatus("u1", &savedStatus{}); found {
		t.Fatal("saved status not deleted")
	}
}

func TestStatusHadNoStatusIsRemovedAfterwards(t *testing.T) {
	mm, s, p := statusFixture(t)
	h := &Huddle{ID: "h1", ChannelID: "c1"}
	p.joined("u1", h)
	if found, _ := s.SavedStatus("u1", &savedStatus{}); !found {
		t.Fatal("'no status' marker must be stored")
	}
	syncUser(t, mm, "u1")
	p.left("u1", h)
	if mm.customStatus["u1"] != nil || len(mm.removedStatus) != 1 {
		t.Fatalf("want status removed, got %+v removed=%v", mm.customStatus["u1"], mm.removedStatus)
	}
}

func TestStatusNotRestoredWhenUserChangedIt(t *testing.T) {
	mm, s, p := statusFixture(t)
	h := &Huddle{ID: "h1", ChannelID: "c1"}
	p.joined("u1", h)
	mine := &model.CustomStatus{Emoji: "coffee", Text: "Break"}
	mm.customStatus["u1"] = mine
	syncUser(t, mm, "u1")
	p.left("u1", h)
	if mm.customStatus["u1"] != mine || len(mm.removedStatus) != 0 {
		t.Fatal("user's own status must stay untouched")
	}
	if found, _ := s.SavedStatus("u1", &savedStatus{}); found {
		t.Fatal("saved status not deleted")
	}
}

func TestStatusExpiredSavedIsCleared(t *testing.T) {
	mm, _, p := statusFixture(t)
	setUserStatus(t, mm, "u1", &model.CustomStatus{Emoji: "x", Text: "old", Duration: "thirty_minutes", ExpiresAt: time.Now().Add(-time.Hour)})
	h := &Huddle{ID: "h1", ChannelID: "c1"}
	p.joined("u1", h)
	syncUser(t, mm, "u1")
	p.left("u1", h)
	if mm.customStatus["u1"] != nil || len(mm.removedStatus) != 1 {
		t.Fatalf("expired status must be removed, got %+v", mm.customStatus["u1"])
	}
}

func TestStatusKeptWhileInAnotherHuddle(t *testing.T) {
	mm, s, p := statusFixture(t)
	h1 := &Huddle{ID: "h1", ChannelID: "c1", Participants: map[string]Participant{"u1": {UserID: "u1", State: StateConfirmed}}}
	h2 := &Huddle{ID: "h2", ChannelID: "c2", Participants: map[string]Participant{"u1": {UserID: "u1", State: StateConfirmed}}}
	for _, h := range []*Huddle{h1, h2} {
		if ok, err := s.CreateHuddle(h); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	p.joined("u1", h1) // in h2 as well: untouched
	if mm.customStatus["u1"] != nil {
		t.Fatal("joined must not set while in another huddle")
	}
	p.left("u1", h1) // still in h2
	if len(mm.removedStatus) != 0 || mm.customStatus["u1"] != nil {
		t.Fatal("left must do nothing while in another huddle")
	}
}

func TestStatusLeftIdempotentAndWithoutJoin(t *testing.T) {
	mm, _, p := statusFixture(t)
	setUserStatus(t, mm, "u1", &model.CustomStatus{Emoji: "palm_tree", Text: "Vacation"})
	h := &Huddle{ID: "h1", ChannelID: "c1"}
	p.left("u1", h) // never joined
	if len(mm.removedStatus) != 0 || mm.customStatus["u1"] != nil {
		t.Fatal("left without joined must do nothing")
	}
	p.joined("u1", h)
	syncUser(t, mm, "u1")
	p.left("u1", h)
	p.left("u1", h) // retry
	if got := mm.customStatus["u1"]; got == nil || got.Emoji != "palm_tree" {
		t.Fatalf("retry clobbered the restore: %+v", got)
	}
}

func TestStatusTextHasNoChannelName(t *testing.T) {
	mm, _, p := statusFixture(t)
	mm.addChannel("c1", model.ChannelTypePrivate, "u1")
	mm.channels["c1"].DisplayName = "secret-project"
	p.joined("u1", &Huddle{ID: "h1", ChannelID: "c1"})
	if got := mm.customStatus["u1"]; got == nil || strings.Contains(got.Text, "secret") || strings.Contains(got.Text, "c1") {
		t.Fatalf("status leaks channel: %+v", got)
	}
}

func TestStatusRestoredAfterConfigChange(t *testing.T) {
	mm := newFakeMM()
	mm.addUser("u1", "Ann", "en")
	s := NewStore(kv.NewMem())
	c := Config{StatusEmoji: "headphones", StatusText: "In a huddle"}
	p := newStatusPresence(mm, s, func() Config { return c })
	setUserStatus(t, mm, "u1", &model.CustomStatus{Emoji: "palm_tree", Text: "Vacation"})
	h := &Huddle{ID: "h1", ChannelID: "c1"}

	p.joined("u1", h)
	syncUser(t, mm, "u1")
	c = Config{StatusEmoji: "phone", StatusText: "On a call"} // admin edits the plugin settings meanwhile
	p.left("u1", h)
	if got := mm.customStatus["u1"]; got == nil || got.Emoji != "palm_tree" || got.Text != "Vacation" {
		t.Fatalf("not restored: %+v", got)
	}
}
