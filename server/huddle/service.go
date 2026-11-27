package huddle

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync/atomic"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
)

// boostFor is how long polling runs at the fast rate after a start, join click or invite.
const boostFor = 60 * time.Second

type Config struct {
	GoogleAccounts                                                    map[string]string // MM user ID → Google email
	FallbackOwnerEmail, StatusText, StatusEmoji, BotUserID, PluginURL string
}

type StartResult struct {
	Huddle         HuddleView
	MeetURL        string
	AlreadyRunning bool
	// Rang: the huddle was just created in a DM/GM and every other member was rung.
	Rang bool
}

// ponytail: ctx params are kept even where unused, for uniform signatures; drop if they never get used.
type ringer interface {
	ringChannel(ctx context.Context, h *Huddle, from string) error
	ringUsers(ctx context.Context, h *Huddle, from string, to []string) error
	expire(ctx context.Context, now time.Time, rings []*Ring)
	settle(userID string, h *Huddle) // userID joined: accept their open rings for h
}

type presence interface {
	joined(userID string, h *Huddle)
	left(userID string, h *Huddle)
}

type Service struct {
	// lastLoadErr: the last TickState error text logged (string), see logLoadErr.
	lastLoadErr atomic.Value
	mm          MM
	meet        google.Meet
	store       Store
	cfg         func() Config
	now         func() time.Time
	ringer      ringer
	presence    presence
}

// NewService wires the production ring and status collaborators.
func NewService(mm MM, meet google.Meet, store Store, cfg func() Config, now func() time.Time) *Service {
	return newService(mm, meet, store, cfg, now, newRingService(mm, store, cfg, now), newStatusPresence(mm, store, cfg))
}

func newService(mm MM, meet google.Meet, store Store, cfg func() Config, now func() time.Time, r ringer, p presence) *Service {
	return &Service{mm: mm, meet: meet, store: store, cfg: cfg, now: now, ringer: r, presence: p}
}

// Start creates a huddle in channelID, or joins the running one (AlreadyRunning).
func (s *Service) Start(ctx context.Context, userID, channelID string) (StartResult, error) {
	if err := s.checkMember(channelID, userID); err != nil {
		return StartResult{}, err
	}
	// ponytail: no per-(user,channel) start cooldown; only authenticated members can start, add one if abused.
	h, err := s.store.Huddle(channelID)
	if err != nil {
		return StartResult{}, err
	}
	running, rang := h != nil, false
	if !running {
		var c created
		if c, err = s.create(ctx, userID, channelID); err != nil {
			return StartResult{}, err
		}
		h, running, rang = c.huddle, c.lost, c.rang
	}
	if running {
		if h, err = s.join(userID, h); err != nil {
			return StartResult{}, err
		}
	}
	return StartResult{Huddle: view(h), MeetURL: s.meetURL(h, userID), AlreadyRunning: running, Rang: rang}, nil
}

// created is create's result: the new huddle, or with lost the winner of a concurrent Start
// (the caller joins it); rang: the DM/GM members were all rung.
type created struct {
	huddle     *Huddle
	lost, rang bool
}

// create starts a new huddle, or returns the winner if a concurrent Start won the create-only CAS.
func (s *Service) create(ctx context.Context, userID, channelID string) (created, error) {
	owner := s.owner(userID)
	if owner == "" {
		return created{}, ErrNoOwner
	}
	ch, appErr := s.mm.GetChannel(channelID)
	if appErr != nil {
		return created{}, appErr
	}
	if ch.DeleteAt != 0 {
		return created{}, ErrForbidden
	}
	u, appErr := s.mm.GetUser(userID)
	if appErr != nil {
		return created{}, appErr
	}
	space, err := s.meet.CreateSpace(ctx, owner)
	if err != nil {
		s.mm.LogError("huddle: create Meet space failed", "channel_id", channelID, "err", err.Error())
		return created{}, ErrMeetUnavailable
	}

	now := s.now()
	h := &Huddle{
		ID: model.NewId(), ChannelID: channelID, Space: space.Name, MeetingURI: space.MeetingURI,
		OwnerEmail: owner, StarterUserID: userID, StarterLocale: u.Locale, StartedAt: now, BoostUntil: now.Add(boostFor),
		Participants: map[string]Participant{userID: {UserID: userID, DisplayName: displayName(u), State: StateJoining, Since: now}},
	}
	ok, err := s.store.CreateHuddle(h)
	if err != nil {
		s.abandon(ctx, owner, space.Name, channelID, err.Error())
		return created{}, err
	}
	if !ok {
		s.abandon(ctx, owner, space.Name, channelID, "lost concurrent start")
		winner, err := s.store.Huddle(channelID)
		if err == nil && winner == nil {
			err = ErrEnded
		}
		return created{huddle: winner, lost: true}, err
	}

	// The card is posted after the CAS so a lost race never leaves a ghost card.
	if post, appErr := s.mm.CreatePost(buildCardPost(h, s.cfg().PluginURL)); appErr != nil {
		s.mm.LogError("huddle: card post failed", "huddle_id", h.ID, "err", appErr.Error())
	} else if fresh, err := s.update(h, func(x *Huddle) error { x.PostID = post.Id; return nil }); err != nil {
		s.mm.LogError("huddle: storing card post failed", "huddle_id", h.ID, "err", err.Error())
	} else {
		h = fresh
		s.updateCard(h) // a join before PostID was stored could not update the card
	}
	s.publish(view(h))
	c := created{huddle: h}
	if ch.IsGroupOrDirect() {
		if err := s.ringer.ringChannel(ctx, h, userID); err != nil {
			s.mm.LogError("huddle: ring channel failed", "huddle_id", h.ID, "err", err.Error())
		} else {
			c.rang = true
		}
	}
	return c, nil
}

// abandon restricts a space no huddle refers to, so its OPEN link cannot be used. Best effort.
func (s *Service) abandon(ctx context.Context, owner, space, channelID, reason string) {
	s.mm.LogError("huddle: abandoning Meet space", "channel_id", channelID, "space", space, "reason", reason)
	if err := s.meet.RestrictSpace(ctx, owner, space); err != nil {
		s.mm.LogError("huddle: restricting abandoned Meet space failed", "space", space, "err", err.Error())
	}
}

func (s *Service) Join(_ context.Context, userID, huddleID string) (string, error) {
	h, err := s.running(huddleID)
	if err != nil {
		return "", err
	}
	if err = s.checkMember(h.ChannelID, userID); err != nil {
		return "", err
	}
	if h, err = s.join(userID, h); err != nil {
		return "", err
	}
	return s.meetURL(h, userID), nil
}

// join marks userID as joining (refreshing an existing joining entry) and boosts polling.
// A confirmed participant is never demoted.
func (s *Service) join(userID string, h *Huddle) (*Huddle, error) {
	u, appErr := s.mm.GetUser(userID)
	if appErr != nil {
		return nil, appErr
	}
	now := s.now()
	var added bool
	h, err := s.update(h, func(x *Huddle) error {
		if !x.EndingSince.IsZero() {
			return ErrEnded
		}
		boost(x, now)
		p, ok := x.Participants[userID]
		added = !ok
		if !ok || p.State == StateJoining {
			x.Participants[userID] = Participant{UserID: userID, DisplayName: displayName(u), State: StateJoining, Since: now}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if added {
		s.updateCard(h)
		s.publish(view(h))
	}
	s.ringer.settle(userID, h)
	return h, nil
}

// Invite rings targetIDs. Self, bots and deactivated users are skipped silently; every other
// target must be an existing user (else ErrBadRequest) and a channel member (else ErrNotMember).
// ErrRateLimited only if all remaining targets are limited; a target whose ring failed is not
// rate limited.
func (s *Service) Invite(ctx context.Context, userID, huddleID string, targetIDs []string) error {
	h, err := s.running(huddleID)
	if err != nil {
		return err
	}
	if err = s.checkMember(h.ChannelID, userID); err != nil {
		return err
	}
	var targets []string
	for _, id := range targetIDs {
		if id == userID {
			continue
		}
		u, appErr := s.mm.GetUser(id)
		if appErr != nil {
			if appErr.StatusCode == http.StatusNotFound || appErr.StatusCode == http.StatusBadRequest {
				return fmt.Errorf("unknown invite target %q: %w", id, ErrBadRequest)
			}
			return appErr
		}
		if ringable(u) {
			targets = append(targets, id)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	// Check every target before TakeRateLimit records anything.
	for _, id := range targets {
		if err = s.checkMember(h.ChannelID, id); errors.Is(err, ErrForbidden) {
			return fmt.Errorf("invite target %q: %w", id, ErrNotMember)
		} else if err != nil {
			return err
		}
	}
	now := s.now()
	var to []string
	for _, id := range targets {
		if s.store.TakeRateLimit(userID, id) {
			to = append(to, id)
		}
	}
	if len(to) == 0 {
		return ErrRateLimited
	}
	if h, err = s.update(h, func(x *Huddle) error { boost(x, now); return nil }); err != nil {
		for _, id := range to {
			s.store.ReleaseRateLimit(userID, id)
		}
		return err
	}
	// One ring per target, so only an undelivered one gives its rate limit back. Fails only if
	// nobody was rung; partial failures are logged.
	var errs []error
	for _, id := range to {
		if rerr := s.ringer.ringUsers(ctx, h, userID, []string{id}); rerr != nil {
			s.store.ReleaseRateLimit(userID, id)
			errs = append(errs, rerr)
		}
	}
	err = errors.Join(errs...)
	if err != nil && len(errs) < len(to) {
		s.mm.LogError("huddle: some invitations failed", "huddle_id", h.ID, "err", err.Error())
		return nil
	}
	return err
}

// End flags the huddle as ending (starter only) and polls it right away: the tick ends the Meet
// conference for everyone (retried every poll), then the huddle once the conference is gone or
// Google kept failing for finishBound.
func (s *Service) End(_ context.Context, userID, huddleID string) error {
	h, err := s.running(huddleID)
	if err != nil {
		return err
	}
	if h.StarterUserID != userID {
		return ErrForbidden
	}
	now := s.now()
	if h, err = s.update(h, func(x *Huddle) error {
		if x.EndingSince.IsZero() {
			x.EndingSince = now
		}
		boost(x, now)
		return nil
	}); err != nil {
		return err
	}
	s.publish(view(h))
	return nil
}

// Active lists the running huddles in channels userID is a member of.
func (s *Service) Active(userID string) ([]HuddleView, error) {
	hs, skipped, err := s.store.ActiveHuddles()
	if err != nil {
		return nil, err
	}
	if skipped != nil {
		s.mm.LogError("huddle: skipping unreadable huddles", "err", skipped.Error()) // the rest still list
	}
	views := []HuddleView{}
	for _, h := range hs {
		if s.checkMember(h.ChannelID, userID) == nil {
			views = append(views, view(h))
		}
	}
	return views, nil
}

// meetURL adds ?authuser= only for users with a configured Google account (never the MM email).
func (s *Service) meetURL(h *Huddle, userID string) string {
	if email := s.cfg().GoogleAccounts[userID]; email != "" {
		return h.MeetingURI + "?authuser=" + url.QueryEscape(email)
	}
	return h.MeetingURI
}

func (s *Service) owner(userID string) string {
	cfg := s.cfg()
	if email := cfg.GoogleAccounts[userID]; email != "" {
		return email
	}
	return cfg.FallbackOwnerEmail
}

func (s *Service) checkMember(channelID, userID string) error {
	_, appErr := s.mm.GetChannelMember(channelID, userID)
	switch {
	case appErr == nil:
		return nil
	case appErr.StatusCode == http.StatusNotFound:
		return ErrForbidden
	}
	return appErr
}

// running returns the huddle for Join/Invite/End; ErrEnded also once its end was requested.
func (s *Service) running(huddleID string) (*Huddle, error) {
	h, err := s.store.HuddleByID(huddleID)
	if err == nil && !h.EndingSince.IsZero() {
		return nil, ErrEnded
	}
	return h, err
}

// boost polls h right away and at the fast rate for boostFor.
func boost(h *Huddle, now time.Time) { h.BoostUntil, h.NextPollAt = now.Add(boostFor), now }

// update applies fn to the stored huddle h; ErrEnded if it is gone or replaced by a newer one.
func (s *Service) update(h *Huddle, fn func(*Huddle) error) (*Huddle, error) {
	fresh, err := s.store.UpdateHuddle(h.ChannelID, func(x *Huddle) error {
		if x.ID != h.ID {
			return ErrEnded
		}
		return fn(x)
	})
	if errors.Is(err, ErrNotFound) {
		return nil, ErrEnded
	}
	return fresh, err
}

func (s *Service) updateCard(h *Huddle) { s.editCard(h, buildCardPost(h, s.cfg().PluginURL)) }

// editCard replaces the card post. Failures (e.g. deleted post) are only logged.
func (s *Service) editCard(h *Huddle, p *model.Post) {
	if h.PostID == "" {
		return
	}
	if _, appErr := s.mm.UpdatePost(p); appErr != nil {
		s.mm.LogError("huddle: card update failed", "huddle_id", h.ID, "post_id", h.PostID, "err", appErr.Error())
	}
}

// publish sends huddle_changed to the channel; the payload carries the HuddleView as a JSON string.
func (s *Service) publish(v HuddleView) {
	b, _ := json.Marshal(v)
	s.mm.PublishWebSocketEvent("huddle_changed", map[string]any{"huddle": string(b)}, &model.WebsocketBroadcast{ChannelId: v.ChannelID})
}

// view converts h for clients; StartedAt is Unix milliseconds, participants ordered by join time.
func view(h *Huddle) HuddleView {
	keys := slices.Collect(maps.Keys(h.Participants))
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(h.Participants[a].Since.Compare(h.Participants[b].Since), cmp.Compare(a, b))
	})
	ps := make([]ParticipantView, 0, len(keys))
	for _, k := range keys {
		p := h.Participants[k]
		ps = append(ps, ParticipantView{UserID: p.UserID, DisplayName: p.DisplayName, State: string(p.State)})
	}
	return HuddleView{ID: h.ID, ChannelID: h.ChannelID, StarterID: h.StarterUserID, StartedAt: h.StartedAt.UnixMilli(), Ending: h.Ending, EndRequested: !h.EndingSince.IsZero(), Participants: ps}
}

func displayName(u *model.User) string { return u.GetDisplayName(model.ShowFullName) }
