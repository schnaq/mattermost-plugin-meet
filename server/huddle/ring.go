package huddle

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	ringTimeout  = 30 * time.Second
	maxReplyLen  = 200
	memberPage   = 200
	ringRinging  = "ringing"
	ringAccepted = "accepted"
	ringDeclined = "declined"
	ringMissed   = "missed"
)

type ringService struct {
	mm    MM
	store Store
	cfg   func() Config
	now   func() time.Time
}

func newRingService(mm MM, store Store, cfg func() Config, now func() time.Time) ringer {
	return &ringService{mm: mm, store: store, cfg: cfg, now: now}
}

// ringChannel rings every other DM/GM member; the card post is the notification.
// ponytail: one GetUser per member; batch with GetUsersByIds if DMs/GMs ever get large.
func (r *ringService) ringChannel(_ context.Context, h *Huddle, from string) error {
	var errs []error
	for page := 0; ; page++ {
		ms, appErr := r.mm.GetChannelMembers(h.ChannelID, page, memberPage)
		if appErr != nil {
			return appErr
		}
		for _, m := range ms {
			if m.UserId == from {
				continue
			}
			if u, appErr := r.mm.GetUser(m.UserId); appErr != nil {
				errs = append(errs, appErr)
				continue
			} else if !ringable(u) {
				continue
			}
			errs = append(errs, r.ring(h, from, m.UserId, h.PostID, false))
		}
		if len(ms) < memberPage {
			return errors.Join(errs...)
		}
	}
}

// ringUsers sends each target a bot DM with a join button.
func (r *ringService) ringUsers(_ context.Context, h *Huddle, from string, to []string) error {
	cfg := r.cfg()
	fu, appErr := r.mm.GetUser(from)
	if appErr != nil {
		return appErr
	}
	ch, appErr := r.mm.GetChannel(h.ChannelID)
	if appErr != nil {
		return appErr
	}
	var errs []error
	for _, id := range to {
		errs = append(errs, r.invite(h, cfg, fu.Username, ch, from, id))
	}
	return errors.Join(errs...)
}

func (r *ringService) invite(h *Huddle, cfg Config, fromName string, ch *model.Channel, from, to string) error {
	tu, appErr := r.mm.GetUser(to)
	if appErr != nil {
		return appErr
	}
	dm, appErr := r.mm.GetDirectChannel(cfg.BotUserID, to)
	if appErr != nil {
		return appErr
	}
	// A DM/GM has no meaningful display name for the invitee; name the channel otherwise.
	msg := fmt.Sprintf(Text(tu.Locale, "invite_dm"), fromName)
	if !ch.IsGroupOrDirect() {
		msg = fmt.Sprintf(Text(tu.Locale, "invite"), fromName, ch.DisplayName)
	}
	p := &model.Post{UserId: cfg.BotUserID, ChannelId: dm.Id, Message: msg}
	p.AddProp("huddle_id", h.ID) // like the card: lets the webapp match the post to the ring
	p.AddProp(model.PostPropsAttachments, []*model.MessageAttachment{{
		Fallback: msg,
		Actions:  []*model.PostAction{JoinAction(tu.Locale, cfg.PluginURL, h.ID)},
	}})
	post, appErr := r.mm.CreatePost(p)
	if appErr != nil {
		return appErr
	}
	return r.ring(h, from, to, post.Id, true)
}

// ringable reports whether u can be rung: not a bot, not deactivated.
func ringable(u *model.User) bool { return !u.IsBot && u.DeleteAt == 0 }

// ring records the Ring and notifies to unless dnd/ooo (the Ring is still kept for the missed hint).
func (r *ringService) ring(h *Huddle, from, to, postID string, targeted bool) error {
	ring := &Ring{ID: model.NewId(), HuddleID: h.ID, ChannelID: h.ChannelID, From: from, To: to, PostID: postID, State: ringRinging, Targeted: targeted, CreatedAt: r.now()}
	if err := r.store.SaveRing(ring); err != nil {
		return err
	}
	if st, appErr := r.mm.GetUserStatus(to); appErr == nil && (st.Status == model.StatusDnd || st.Status == model.StatusOutOfOffice) {
		return nil
	}
	r.mm.PublishWebSocketEvent("ring", map[string]any{"ring_id": ring.ID, "huddle_id": h.ID, "channel_id": h.ChannelID, "from": from}, &model.WebsocketBroadcast{UserId: to})
	return nil
}

// expire turns rings older than ringTimeout into missed and cleans up finished ones.
// ponytail: each missed ring re-reads its huddle; group by huddle if many rings expire at once.
func (r *ringService) expire(_ context.Context, now time.Time, rings []*Ring) {
	for _, ring := range rings {
		if now.Sub(ring.CreatedAt) <= ringTimeout {
			continue
		}
		if ring.State == ringRinging {
			// rings is the tick's snapshot from before the polls: re-read so a ring answered meanwhile is not missed.
			fresh, err := r.store.Ring(ring.ID)
			if err != nil { // keep the ring: deleting it now would lose the missed notice
				r.mm.LogError("huddle: re-reading ring failed", "ring_id", ring.ID, "err", err.Error())
				continue
			}
			if fresh != nil && fresh.State == ringRinging {
				r.miss(fresh)
			}
		}
		if err := r.store.DeleteRing(ring.ID); err != nil {
			r.mm.LogError("huddle: deleting ring failed", "ring_id", ring.ID, "err", err.Error())
		}
	}
}

// involved reports whether userID is or was a participant of h.
func involved(h *Huddle, userID string) bool {
	_, ok := h.Participants[userID]
	return ok || slices.Contains(h.SeenUserIDs, userID)
}

// others reports whether anyone but the starter is or was a participant of h.
func others(h *Huddle) bool {
	notStarter := func(k string) bool { return k != h.StarterUserID }
	return slices.ContainsFunc(h.SeenUserIDs, notStarter) || slices.ContainsFunc(slices.Collect(maps.Keys(h.Participants)), notStarter)
}

// settle accepts userID's open rings for h (they joined without answering the ring).
func (r *ringService) settle(userID string, h *Huddle) {
	rings, err := r.store.Rings()
	if err != nil {
		r.mm.LogError("huddle: listing rings failed", "err", err.Error())
		return
	}
	for _, ring := range rings {
		if ring.HuddleID != h.ID || ring.To != userID || ring.State != ringRinging {
			continue
		}
		ring.State = ringAccepted
		if err := r.store.SaveRing(ring); err != nil {
			r.mm.LogError("huddle: settling ring failed", "ring_id", ring.ID, "err", err.Error())
			continue
		}
		claimHint(r.mm, r.store, h.ID)
		publishRingEnded(r.mm, ring)
	}
}

func (r *ringService) miss(ring *Ring) {
	h, _ := r.store.HuddleByID(ring.HuddleID)
	if h != nil && involved(h, ring.To) {
		ring.State = ringAccepted // already in the huddle: not a missed call
		publishRingEnded(r.mm, ring)
		return
	}
	ring.State = ringMissed
	publishRingEnded(r.mm, ring)
	publishRingOutcome(r.mm, ring)
	if ring.Targeted {
		r.updateMissedDM(ring)
		return
	}
	if h != nil && others(h) {
		return
	}
	h, claimed := claimHint(r.mm, r.store, ring.HuddleID)
	if !claimed {
		return
	}
	p := &model.Post{UserId: h.StarterUserID, ChannelId: h.ChannelID, RootId: h.PostID, Message: Text(Locale(r.mm.GetUser, h.StarterUserID), "missed")}
	if _, appErr := r.mm.CreatePost(p); appErr != nil {
		r.mm.LogError("huddle: missed hint failed", "huddle_id", h.ID, "err", appErr.Error())
	}
}

func (r *ringService) updateMissedDM(ring *Ring) {
	cfg := r.cfg()
	dm, appErr := r.mm.GetDirectChannel(cfg.BotUserID, ring.To)
	if appErr != nil {
		r.mm.LogError("huddle: missed dm failed", "ring_id", ring.ID, "err", appErr.Error())
		return
	}
	var fromName string
	if u, appErr := r.mm.GetUser(ring.From); appErr == nil {
		fromName = u.Username
	}
	p := &model.Post{Id: ring.PostID, UserId: cfg.BotUserID, ChannelId: dm.Id, Message: fmt.Sprintf(Text(Locale(r.mm.GetUser, ring.To), "missed_from"), fromName)}
	p.AddProp(model.PostPropsAttachments, []*model.MessageAttachment{}) // drop the Join button
	if _, appErr := r.mm.UpdatePost(p); appErr != nil {
		r.mm.LogError("huddle: missed dm update failed", "ring_id", ring.ID, "err", appErr.Error())
	}
}

// claimHint sets Huddle.RingSettled; claimed is true only for the caller that flipped it. A
// missing or ended huddle yields claimed=false.
func claimHint(mm MM, store Store, huddleID string) (h *Huddle, claimed bool) {
	h, err := store.HuddleByID(huddleID)
	if err != nil {
		return nil, false
	}
	h, err = store.UpdateHuddle(h.ChannelID, func(x *Huddle) error {
		if x.ID != huddleID {
			return ErrEnded
		}
		claimed = !x.RingSettled
		x.RingSettled = true
		return nil
	})
	if err != nil {
		if !errors.Is(err, ErrEnded) && !errors.Is(err, ErrNotFound) {
			mm.LogError("huddle: claiming missed hint failed", "huddle_id", huddleID, "err", err.Error())
		}
		return nil, false
	}
	return h, claimed
}

// publishRingEnded tells both sides the ring is over (closes the callee's dialog).
func publishRingEnded(mm MM, ring *Ring) {
	payload := map[string]any{"ring_id": ring.ID, "state": ring.State}
	for _, u := range []string{ring.To, ring.From} {
		mm.PublishWebSocketEvent("ring_ended", payload, &model.WebsocketBroadcast{UserId: u})
	}
}

// publishRingOutcome tells the caller their ring was declined or missed (the toast). A quick
// reply sends none: it arrives as a DM instead.
func publishRingOutcome(mm MM, ring *Ring) {
	payload := map[string]any{"ring_id": ring.ID, "state": ring.State, "to": ring.To}
	mm.PublishWebSocketEvent("ring_outcome", payload, &model.WebsocketBroadcast{UserId: ring.From})
}

// RespondRing handles the callee's answer: accept|decline|quickreply and returns the ring's
// huddle ID. Accept only records the state; the API layer calls Join for the Meet URL.
// Only Ring.To may respond, once.
// ponytail: accept does not Join here; the API layer joins and owns the Meet URL.
func (s *Service) RespondRing(_ context.Context, userID, ringID, action, reply string) (string, error) {
	ring, err := s.store.Ring(ringID)
	if err != nil {
		return "", err
	}
	if ring == nil {
		return "", ErrNotFound
	}
	if ring.To != userID {
		return "", ErrForbidden
	}
	if ring.State != ringRinging {
		return "", ErrEnded
	}
	var state string
	switch action {
	case "accept":
		state = ringAccepted
		// Checked before the ring changes, so the callee sees "ended" and nothing is half-accepted.
		if _, err = s.running(ring.HuddleID); err != nil {
			return "", err
		}
	case "decline":
		state = ringDeclined
	case "quickreply":
		state = ringDeclined
		if reply = truncateRunes(strings.TrimSpace(reply), maxReplyLen); reply == "" {
			return "", fmt.Errorf("empty quick reply: %w", ErrBadRequest)
		}
		// Posted before the state is saved: a failed post leaves the ring ringing, so a retry works.
		// ponytail: a failed SaveRing after a sent reply lets a retry post it twice; rare and harmless.
		dm, appErr := s.mm.GetDirectChannel(ring.To, ring.From)
		if appErr != nil {
			return "", appErr
		}
		if _, appErr = s.mm.CreatePost(&model.Post{UserId: ring.To, ChannelId: dm.Id, Message: reply}); appErr != nil {
			return "", appErr
		}
	default:
		return "", fmt.Errorf("unknown ring action: %w", ErrBadRequest)
	}
	// ponytail: no CAS on the ring; two simultaneous responses by the callee may both win.
	ring.State = state
	if err = s.store.SaveRing(ring); err != nil {
		return "", err
	}
	if state == ringAccepted {
		claimHint(s.mm, s.store, ring.HuddleID) // an accepted call needs no missed hint
	}
	publishRingEnded(s.mm, ring)
	if action == "decline" {
		publishRingOutcome(s.mm, ring)
	}
	return ring.HuddleID, nil
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
