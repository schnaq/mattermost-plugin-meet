package huddle

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
)

const (
	// joiningTimeout is how long a rung user may stay "joining" without showing up in Meet.
	joiningTimeout = 2 * time.Minute
	// leaveGrace is how long a confirmed participant may be absent from Meet before counting as left.
	leaveGrace = 20 * time.Second
)

// Resolver maps a Google user ID to a Mattermost user ID.
// ponytail: a func, not a map; the tick passes a map lookup, swap if no other resolver appears.
type Resolver func(googleUserID string) (mmUserID string, ok bool)

// MergeResult: name correlation stays huddle-local; only Directory lookups are cached (gid:).
type MergeResult struct {
	Joined, Left []Participant // copies, in participant key order
}

// Merge reconciles h.Participants (mutated in place) with the current Meet
// participant list. No I/O; a second call with the same input reports nothing.
func Merge(h *Huddle, gps []google.Participant, resolve Resolver, names map[string]string, now time.Time) MergeResult {
	if h.Participants == nil {
		h.Participants = map[string]Participant{}
	}
	var r MergeResult
	var joined []string // keys
	claimed := map[string]bool{}

	confirm := func(key string, p Participant, g google.Participant) {
		if p.State != StateConfirmed {
			p.State, p.Since = StateConfirmed, now
			joined = append(joined, key)
		}
		p.MissingSince = time.Time{}
		p.GoogleUserID, p.GoogleDisplayName = g.GoogleUserID, g.DisplayName
		h.Participants[key] = p
		claimed[key] = true
		if p.UserID != "" && !slices.Contains(h.SeenUserIDs, p.UserID) {
			h.SeenUserIDs = append(h.SeenUserIDs, p.UserID)
		}
	}

	// Pass 1: Google users that resolve to a Mattermost user claim their key first.
	var unresolved []google.Participant
	for _, g := range gps {
		uid, ok := "", false
		if g.GoogleUserID != "" {
			uid, ok = resolve(g.GoogleUserID)
		}
		if !ok {
			unresolved = append(unresolved, g)
			continue
		}
		if claimed[uid] { // same user on a second device
			continue
		}
		p := h.Participants[uid]
		p.UserID = uid
		if n := names[uid]; n != "" {
			p.DisplayName = n
		} else if p.DisplayName == "" {
			p.DisplayName = g.DisplayName
		}
		confirm(uid, p, g)
	}

	// Pass 2: unresolved (signed external or anonymous) participants.
	for _, g := range unresolved {
		keys := slices.Sorted(maps.Keys(h.Participants))

		// Already confirmed on an earlier tick (possibly correlated to an MM user).
		// Anonymous guests with identical names are indistinguishable; they may swap keys.
		known := slices.IndexFunc(keys, func(k string) bool {
			p := h.Participants[k]
			return !claimed[k] && p.State == StateConfirmed && p.GoogleUserID == g.GoogleUserID &&
				(g.GoogleUserID != "" || p.GoogleDisplayName == g.DisplayName)
		})
		if known >= 0 {
			confirm(keys[known], h.Participants[keys[known]], g)
			continue
		}

		// Exactly one rung user with a matching name → that's them.
		var match []string
		for _, k := range keys {
			if p := h.Participants[k]; p.State == StateJoining && namesMatch(p.DisplayName, g.DisplayName) {
				match = append(match, k)
			}
		}
		if len(match) == 1 {
			confirm(match[0], h.Participants[match[0]], g)
			continue
		}

		// Unknown guest. Same display name twice → suffix "#2", "#3", ...
		base := "a:" + g.DisplayName
		if g.GoogleUserID != "" {
			base = "g:" + g.GoogleUserID
		}
		key := base
		for i := 2; claimed[key]; i++ {
			key = base + "#" + strconv.Itoa(i)
		}
		confirm(key, Participant{DisplayName: g.DisplayName}, g)
	}

	for _, k := range slices.Sorted(maps.Keys(h.Participants)) {
		p := h.Participants[k]
		switch {
		case claimed[k]:
			continue
		case p.State == StateJoining:
			if now.Sub(p.Since) <= joiningTimeout {
				continue
			}
		case p.MissingSince.IsZero():
			p.MissingSince = now
			h.Participants[k] = p
			continue
		case now.Sub(p.MissingSince) < leaveGrace:
			continue
		}
		delete(h.Participants, k)
		r.Left = append(r.Left, p)
	}
	slices.Sort(joined)
	for _, k := range joined {
		r.Joined = append(r.Joined, h.Participants[k])
	}
	return r
}

// namesMatch reports whether the Mattermost name equals the Google name after
// normalization (lowercase, diacritics stripped, whitespace collapsed) or
// contains it as a whole-word sequence ("anna muller" matches "anna", not "ann").
// Empty names never match.
func namesMatch(mmName, googleName string) bool {
	mm, g := normalizeName(mmName), normalizeName(googleName)
	return g != "" && strings.Contains(" "+mm+" ", " "+g+" ")
}

func normalizeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFD.String(s))
	return strings.Join(strings.Fields(s), " ")
}
