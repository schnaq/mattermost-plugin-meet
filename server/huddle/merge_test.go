package huddle

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
)

var t0 = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

func resolverOf(m map[string]string) Resolver {
	return func(gid string) (string, bool) {
		id, ok := m[gid]
		return id, ok
	}
}

var noResolve = resolverOf(nil)

func joining(uid, name string, since time.Time) Participant {
	return Participant{UserID: uid, DisplayName: name, State: StateJoining, Since: since}
}

func newHuddle(ps ...Participant) *Huddle {
	h := &Huddle{Participants: map[string]Participant{}}
	for _, p := range ps {
		h.Participants[p.UserID] = p
	}
	return h
}

func keys(h *Huddle) []string {
	ks := make([]string, 0, len(h.Participants))
	for k := range h.Participants {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

// who names merged participants: UserID, else (guests) DisplayName.
func who(ps []Participant) []string {
	var out []string
	for _, p := range ps {
		out = append(out, cmp.Or(p.UserID, p.DisplayName))
	}
	return out
}

func assertStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %q, want %q", what, got, want)
	}
}

func TestMergeConfirmsResolvedUser(t *testing.T) {
	h := &Huddle{} // nil Participants must be handled
	gps := []google.Participant{{GoogleUserID: "users/1", DisplayName: "Bob G"}}
	r := Merge(h, gps, resolverOf(map[string]string{"users/1": "bob"}), map[string]string{"bob": "Bob"}, t0)

	assertStrings(t, "Joined", who(r.Joined), []string{"bob"})
	assertStrings(t, "Left", who(r.Left), nil)
	p := h.Participants["bob"]
	if p.State != StateConfirmed || p.UserID != "bob" || p.DisplayName != "Bob" || p.GoogleUserID != "users/1" || !p.Since.Equal(t0) {
		t.Fatalf("participant = %+v", p)
	}
	assertStrings(t, "SeenUserIDs", h.SeenUserIDs, []string{"bob"})
}

func TestMergeKeepsJoiningUnderTwoMinutes(t *testing.T) {
	h := newHuddle(joining("anna", "Anna", t0.Add(-time.Minute)))
	r := Merge(h, nil, noResolve, nil, t0)

	assertStrings(t, "Left", who(r.Left), nil)
	assertStrings(t, "keys", keys(h), []string{"anna"})
	if h.Participants["anna"].State != StateJoining {
		t.Fatalf("state = %v", h.Participants["anna"].State)
	}
}

func TestMergeDropsJoiningAfterTwoMinutes(t *testing.T) {
	h := newHuddle(joining("anna", "Anna", t0.Add(-3*time.Minute)))
	r := Merge(h, nil, noResolve, nil, t0)

	assertStrings(t, "Left", who(r.Left), []string{"anna"})
	assertStrings(t, "keys", keys(h), []string{})
}

func TestMergeRemovesLeftUser(t *testing.T) {
	h := &Huddle{}
	res := resolverOf(map[string]string{"users/1": "bob", "users/2": "carl"})
	Merge(h, []google.Participant{{GoogleUserID: "users/1"}, {GoogleUserID: "users/2"}}, res, nil, t0)
	r := Merge(h, []google.Participant{{GoogleUserID: "users/2"}}, res, nil, t0.Add(5*time.Second))
	assertStrings(t, "Left (grace)", who(r.Left), nil)
	assertStrings(t, "keys (grace)", keys(h), []string{"bob", "carl"})

	r = Merge(h, []google.Participant{{GoogleUserID: "users/2"}}, res, nil, t0.Add(25*time.Second))
	assertStrings(t, "Left", who(r.Left), []string{"bob"})
	assertStrings(t, "Joined", who(r.Joined), nil)
	assertStrings(t, "keys", keys(h), []string{"carl"})
	assertStrings(t, "SeenUserIDs", h.SeenUserIDs, []string{"bob", "carl"})
}

func TestMergeCorrelatesAnonymousByName(t *testing.T) {
	h := newHuddle(joining("anna", "Anna Müller", t0))
	r := Merge(h, []google.Participant{{DisplayName: "anna  muller", Anonymous: true}}, noResolve, nil, t0)

	assertStrings(t, "Joined", who(r.Joined), []string{"anna"})
	assertStrings(t, "keys", keys(h), []string{"anna"})
	p := h.Participants["anna"]
	if p.State != StateConfirmed || p.GoogleDisplayName != "anna  muller" || p.DisplayName != "Anna Müller" {
		t.Fatalf("participant = %+v", p)
	}
	assertStrings(t, "SeenUserIDs", h.SeenUserIDs, []string{"anna"})
}

func TestMergeCorrelatesSignedExternalByName(t *testing.T) {
	for name, gname := range map[string]string{"exact": " ANNA  muller", "containment": "Anna"} {
		t.Run(name, func(t *testing.T) {
			h := newHuddle(joining("anna", "Anna Müller", t0))
			r := Merge(h, []google.Participant{{GoogleUserID: "users/9", DisplayName: gname}}, noResolve, nil, t0)

			assertStrings(t, "Joined", who(r.Joined), []string{"anna"})
			if p := h.Participants["anna"]; p.State != StateConfirmed || p.GoogleUserID != "users/9" {
				t.Fatalf("participant = %+v", p)
			}
		})
	}
}

func TestMergeNoCorrelationWithTwoJoining(t *testing.T) {
	h := newHuddle(joining("anna1", "Anna Müller", t0), joining("anna2", "Anna Schmidt", t0))
	r := Merge(h, []google.Participant{{DisplayName: "Anna", Anonymous: true}}, noResolve, nil, t0)

	assertStrings(t, "Joined", who(r.Joined), []string{"Anna"})
	assertStrings(t, "keys", keys(h), []string{"a:Anna", "anna1", "anna2"})
	if p := h.Participants["a:Anna"]; p.State != StateConfirmed || p.UserID != "" || p.DisplayName != "Anna" {
		t.Fatalf("participant = %+v", p)
	}
	assertStrings(t, "SeenUserIDs", h.SeenUserIDs, nil)
}

func TestMergeKeepsCorrelatedAnonymousOnNextTick(t *testing.T) {
	h := newHuddle(joining("anna", "Anna Müller", t0))
	gps := []google.Participant{{DisplayName: "anna muller", Anonymous: true}}
	Merge(h, gps, noResolve, nil, t0)
	r := Merge(h, gps, noResolve, nil, t0.Add(5*time.Second))

	assertStrings(t, "Joined", who(r.Joined), nil)
	assertStrings(t, "Left", who(r.Left), nil)
	assertStrings(t, "keys", keys(h), []string{"anna"})
	if p := h.Participants["anna"]; p.State != StateConfirmed || !p.Since.Equal(t0) {
		t.Fatalf("participant = %+v", p)
	}
	assertStrings(t, "SeenUserIDs", h.SeenUserIDs, []string{"anna"})
}

func TestMergeKeepsCorrelatedSignedExternalOnNextTick(t *testing.T) {
	gps := []google.Participant{{GoogleUserID: "users/9", DisplayName: "Anna"}}
	for name, res := range map[string]Resolver{
		"mapping not yet persisted": noResolve,
		"mapping persisted":         resolverOf(map[string]string{"users/9": "anna"}),
	} {
		t.Run(name, func(t *testing.T) {
			h := newHuddle(joining("anna", "Anna Müller", t0))
			Merge(h, gps, noResolve, nil, t0)
			r := Merge(h, gps, res, nil, t0.Add(5*time.Second))

			assertStrings(t, "Joined", who(r.Joined), nil)
			assertStrings(t, "Left", who(r.Left), nil)
			assertStrings(t, "keys", keys(h), []string{"anna"})
			if p := h.Participants["anna"]; p.DisplayName != "Anna Müller" || !p.Since.Equal(t0) {
				t.Fatalf("participant = %+v", p)
			}
		})
	}
}

func TestMergeIsIdempotent(t *testing.T) {
	h := newHuddle(joining("anna", "Anna", t0), joining("eve", "Eve", t0))
	gps := []google.Participant{
		{GoogleUserID: "users/1", DisplayName: "Bob"},
		{DisplayName: "Anna", Anonymous: true},
		{GoogleUserID: "users/7", DisplayName: "Zed Extern"},
		{DisplayName: "Guest", Anonymous: true},
	}
	res := resolverOf(map[string]string{"users/1": "bob"})
	r1 := Merge(h, gps, res, nil, t0)
	assertStrings(t, "Joined 1", who(r1.Joined), []string{"Guest", "anna", "bob", "Zed Extern"})

	r2 := Merge(h, gps, res, nil, t0.Add(5*time.Second))
	if len(r2.Joined)+len(r2.Left) != 0 {
		t.Fatalf("second merge = %+v", r2)
	}
	assertStrings(t, "keys", keys(h), []string{"a:Guest", "anna", "bob", "eve", "g:users/7"})
}

func TestMergeSameAnonymousNameGetsDistinctKeys(t *testing.T) {
	h := &Huddle{}
	gps := []google.Participant{{DisplayName: "Gast", Anonymous: true}, {DisplayName: "Gast", Anonymous: true}}
	r1 := Merge(h, gps, noResolve, nil, t0)
	assertStrings(t, "Joined", who(r1.Joined), []string{"Gast", "Gast"})

	r2 := Merge(h, gps, noResolve, nil, t0.Add(5*time.Second))
	assertStrings(t, "Joined 2", who(r2.Joined), nil)
	assertStrings(t, "Left 2", who(r2.Left), nil)

	r3 := Merge(h, gps[:1], noResolve, nil, t0.Add(10*time.Second))
	assertStrings(t, "Left 3", who(r3.Left), nil)
	r4 := Merge(h, gps[:1], noResolve, nil, t0.Add(30*time.Second))
	assertStrings(t, "Left 4", who(r4.Left), []string{"Gast"})
	assertStrings(t, "keys", keys(h), []string{"a:Gast"})
}

func TestMergeSameUserOnTwoDevicesJoinsOnce(t *testing.T) {
	h := &Huddle{}
	gps := []google.Participant{{GoogleUserID: "users/1"}, {GoogleUserID: "users/1"}}
	r := Merge(h, gps, resolverOf(map[string]string{"users/1": "bob"}), nil, t0)

	assertStrings(t, "Joined", who(r.Joined), []string{"bob"})
	assertStrings(t, "keys", keys(h), []string{"bob"})
	assertStrings(t, "SeenUserIDs", h.SeenUserIDs, []string{"bob"})
}

func TestMergeBriefDisappearanceNoFlicker(t *testing.T) {
	h := &Huddle{}
	res := resolverOf(map[string]string{"users/1": "bob"})
	gps := []google.Participant{{GoogleUserID: "users/1"}, {DisplayName: "Gast", Anonymous: true}}
	Merge(h, gps, res, nil, t0)

	r := Merge(h, nil, res, nil, t0.Add(5*time.Second))
	assertStrings(t, "Left (missing)", who(r.Left), nil)
	if h.Participants["bob"].MissingSince.IsZero() {
		t.Fatal("MissingSince not set")
	}

	r = Merge(h, gps, res, nil, t0.Add(10*time.Second))
	assertStrings(t, "Joined (back)", who(r.Joined), nil)
	assertStrings(t, "Left (back)", who(r.Left), nil)
	assertStrings(t, "keys", keys(h), []string{"a:Gast", "bob"})
	for k, p := range h.Participants {
		if !p.MissingSince.IsZero() || !p.Since.Equal(t0) {
			t.Fatalf("%s = %+v", k, p)
		}
	}
}

func TestMergeLeftAfterGrace(t *testing.T) {
	h := &Huddle{}
	gps := []google.Participant{{DisplayName: "Gast", Anonymous: true}}
	Merge(h, gps, noResolve, nil, t0)
	Merge(h, nil, noResolve, nil, t0.Add(5*time.Second))

	r := Merge(h, nil, noResolve, nil, t0.Add(24*time.Second))
	assertStrings(t, "Left (19s)", who(r.Left), nil)
	r = Merge(h, nil, noResolve, nil, t0.Add(25*time.Second))
	assertStrings(t, "Left (20s)", who(r.Left), []string{"Gast"})
	assertStrings(t, "keys", keys(h), []string{})
}

func TestNamesMatch(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Anna Müller", "anna muller", true},
		{"Anna Müller", "Anna", true},  // whole-word containment, either direction
		{"Anna", "Anna Müller", false}, // Google name must not be longer than MM name
		{"  José   García ", "jose garcia", true},
		{"Anna Müller", "Ann", false}, // word boundary
		{"Anna Müller", "nna", false},
		{"Anna", "", false}, // empty never matches
		{"", "", false},
		{"Anna Müller", "Bob", false},
	} {
		if got := namesMatch(c.a, c.b); got != c.want {
			t.Errorf("namesMatch(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
