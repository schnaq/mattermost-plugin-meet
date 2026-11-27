package huddle

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/schnaq/mattermost-plugin-meet/server/kv"
)

// Keys: h:<channelID>, hid:<huddleID>, st:<userID>, gid:<googleUserID>, gx:<googleUserID>,
// r:<ringID>, rl:<from>:<to>, fu:<huddleID>.
const (
	pHuddle   = "h:"
	pID       = "hid:"
	pStatus   = "st:"
	pGoogle   = "gid:"
	pNoGoogle = "gx:"
	pRing     = "r:"
	pRate     = "rl:"
	pFollowup = "fu:"

	maxCAS    = 5
	rateLimit = 60 * time.Second
	// mappingTTL bounds how long a Directory-resolved gid: mapping is trusted.
	mappingTTL = 24 * time.Hour
	// endedTTL is how long an ended huddle's ID still answers ErrEnded (then ErrNotFound).
	endedTTL = 7 * 24 * time.Hour
)

type Store struct{ kv kv.KV }

func NewStore(k kv.KV) Store { return Store{k} }

type idEntry struct {
	ChannelID string
	Ended     bool
}

// CreateHuddle stores h unless the channel already has a huddle (false). hid: is written
// first, so a stored h: is always reachable by ID; a failed or lost create drops it again.
func (s Store) CreateHuddle(h *Huddle) (bool, error) {
	if err := s.kv.Set(pID+h.ID, idEntry{ChannelID: h.ChannelID}); err != nil {
		return false, err
	}
	ok, err := s.kv.CompareAndSet(pHuddle+h.ChannelID, nil, h)
	if err != nil || !ok {
		_ = s.kv.Delete(pID + h.ID) // best effort: a leftover entry only answers ErrEnded
	}
	return ok, err
}

func (s Store) Huddle(channelID string) (*Huddle, error) {
	var h Huddle
	found, err := s.kv.Get(pHuddle+channelID, &h)
	if err != nil || !found {
		return nil, err
	}
	return &h, nil
}

// HuddleByID returns the active huddle, ErrEnded if it ended (or was replaced), ErrNotFound if unknown.
func (s Store) HuddleByID(huddleID string) (*Huddle, error) {
	var e idEntry
	found, err := s.kv.Get(pID+huddleID, &e)
	switch {
	case err != nil:
		return nil, err
	case !found:
		return nil, ErrNotFound
	case e.Ended:
		return nil, ErrEnded
	}
	h, err := s.Huddle(e.ChannelID)
	if err != nil {
		return nil, err
	}
	if h == nil || h.ID != huddleID {
		return nil, ErrEnded
	}
	return h, nil
}

// UpdateHuddle applies fn to a fresh copy and stores it via CAS, retrying up to 5 times.
// Returns ErrNotFound if there is no huddle for the channel.
// ponytail: own CAS loop over kv.KV, not pluginapi SetAtomicWithRetries, so the mem KV covers it in tests.
func (s Store) UpdateHuddle(channelID string, fn func(*Huddle) error) (*Huddle, error) {
	key := pHuddle + channelID
	for range maxCAS {
		var raw json.RawMessage
		found, err := s.kv.Get(key, &raw)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrNotFound
		}
		var h Huddle
		if err = json.Unmarshal(raw, &h); err != nil {
			return nil, err
		}
		if err = fn(&h); err != nil {
			return nil, err
		}
		ok, err := s.kv.CompareAndSet(key, raw, &h)
		if err != nil {
			return nil, err
		}
		if ok {
			return &h, nil
		}
	}
	return nil, fmt.Errorf("update huddle %s: conflict after %d attempts", channelID, maxCAS)
}

// EndHuddle marks the ID ended before deleting the huddle, so a failure leaves it active
// (and retried by the next tick) instead of half-ended.
func (s Store) EndHuddle(h *Huddle) error {
	if err := s.kv.SetWithTTL(pID+h.ID, idEntry{ChannelID: h.ChannelID, Ended: true}, endedTTL); err != nil {
		return err
	}
	return s.kv.Delete(pHuddle + h.ChannelID)
}

// load loads the values of the keys with prefix; keys deleted meanwhile are skipped. A key
// that fails to load is skipped too: the others are returned with an error naming it.
func load[T any](k kv.KV, keys []string, prefix string) ([]*T, error) {
	var out []*T
	var errs []error
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		v := new(T)
		found, err := k.Get(key, v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", key, err))
			continue
		}
		if found {
			out = append(out, v)
		}
	}
	return out, errors.Join(errs...)
}

// TickState loads the huddles, rings and follow-ups with a single key scan: each ListKeys pages
// through the whole plugin keyspace, and the tick runs every 2 s. Entries that fail to load are
// skipped and reported in err; a failed key scan returns nothing.
func (s Store) TickState() (hs []*Huddle, rs []*Ring, fs []*Followup, err error) {
	keys, err := s.kv.ListKeys("")
	if err != nil {
		return nil, nil, nil, err
	}
	hs, herr := load[Huddle](s.kv, keys, pHuddle)
	rs, rerr := load[Ring](s.kv, keys, pRing)
	fs, ferr := load[Followup](s.kv, keys, pFollowup)
	return hs, rs, fs, errors.Join(herr, rerr, ferr)
}

// ActiveHuddles fails (err) only if the key scan failed; skipped names unreadable entries,
// which are left out of hs.
func (s Store) ActiveHuddles() (hs []*Huddle, skipped, err error) {
	keys, err := s.kv.ListKeys(pHuddle)
	if err != nil {
		return nil, nil, err
	}
	hs, skipped = load[Huddle](s.kv, keys, pHuddle)
	return hs, skipped, nil
}

func (s Store) SaveRing(r *Ring) error { return s.kv.Set(pRing+r.ID, r) }

func (s Store) Ring(id string) (*Ring, error) {
	var r Ring
	found, err := s.kv.Get(pRing+id, &r)
	if err != nil || !found {
		return nil, err
	}
	return &r, nil
}

func (s Store) Rings() ([]*Ring, error) {
	keys, err := s.kv.ListKeys(pRing)
	if err != nil {
		return nil, err
	}
	return load[Ring](s.kv, keys, pRing)
}

func (s Store) DeleteRing(id string) error { return s.kv.Delete(pRing + id) }

func (s Store) SaveFollowup(f *Followup) error { return s.kv.Set(pFollowup+f.HuddleID, f) }

// Followup returns the follow-up for huddleID, nil if none.
func (s Store) Followup(huddleID string) (*Followup, error) {
	var f Followup
	found, err := s.kv.Get(pFollowup+huddleID, &f)
	if err != nil || !found {
		return nil, err
	}
	return &f, nil
}

func (s Store) DeleteFollowup(huddleID string) error { return s.kv.Delete(pFollowup + huddleID) }

// SaveStatus stores the user's pre-huddle MM status (JSON, e.g. a custom status or a "no status" marker).
func (s Store) SaveStatus(userID string, v any) error { return s.kv.Set(pStatus+userID, v) }

// SavedStatus loads the saved status into out; found=false if none was saved.
func (s Store) SavedStatus(userID string, out any) (found bool, err error) {
	return s.kv.Get(pStatus+userID, out)
}

func (s Store) DeleteStatus(userID string) error { return s.kv.Delete(pStatus + userID) }

// GoogleMapping returns the MM user for a Google user ID ("" if unknown).
func (s Store) GoogleMapping(googleUserID string) (string, error) {
	var u string
	_, err := s.kv.Get(pGoogle+googleUserID, &u)
	return u, err
}

func (s Store) SaveGoogleMapping(googleUserID, userID string) error {
	return s.kv.SetWithTTL(pGoogle+googleUserID, userID, mappingTTL)
}

// GoogleMiss reports whether googleUserID failed to resolve to an MM user within missTTL.
func (s Store) GoogleMiss(googleUserID string) (bool, error) {
	return s.kv.Get(pNoGoogle+googleUserID, new(bool))
}

func (s Store) SaveGoogleMiss(googleUserID string) error {
	return s.kv.SetWithTTL(pNoGoogle+googleUserID, true, missTTL)
}

// ClearGoogleCache drops every gid:/gx: entry (e.g. after the Google configuration changed),
// with a single key scan.
func (s Store) ClearGoogleCache() error {
	keys, err := s.kv.ListKeys("")
	if err != nil {
		return err
	}
	var errs []error
	for _, k := range keys {
		if strings.HasPrefix(k, pGoogle) || strings.HasPrefix(k, pNoGoogle) {
			errs = append(errs, s.kv.Delete(k))
		}
	}
	return errors.Join(errs...)
}

// TakeRateLimit records a ring from->to and reports false if there was one within rateLimit.
// KV errors fail open (true).
// ponytail: read-then-write, not atomic; a racing double ring is harmless.
func (s Store) TakeRateLimit(from, to string) bool {
	key := pRate + from + ":" + to
	if limited, err := s.kv.Get(key, new(bool)); err == nil && limited {
		return false
	}
	_ = s.kv.SetWithTTL(key, true, rateLimit)
	return true
}

// ReleaseRateLimit undoes TakeRateLimit after a ring that was not delivered. Best effort.
func (s Store) ReleaseRateLimit(from, to string) { _ = s.kv.Delete(pRate + from + ":" + to) }
