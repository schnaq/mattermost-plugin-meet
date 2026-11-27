package huddle

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
)

const (
	pollFast = 4 * time.Second
	pollSlow = 15 * time.Second
	// noConference ends a huddle nobody ever joined in Meet.
	noConference = 10 * time.Minute
	// maxHuddle is the safety cap for forgotten tabs.
	maxHuddle = 12 * time.Hour
	// missTTL is how long a Google user that did not resolve to an MM user is not looked up again.
	missTTL       = time.Hour
	followupEvery = 5 * time.Minute
	followupFor   = 2 * time.Hour
	// finishBound: after this long finishing, Meet/follow-up failures no longer block the end.
	finishBound = 5 * time.Minute
	// endGrace: how long a seen conference may be gone before the huddle ends (tab reload,
	// brief drop, or Meet rolling over to a new conference in the same space).
	endGrace = 30 * time.Second
)

// googleTimeout bounds each poll's, finish's and follow-up's Google calls (a var for tests).
var googleTimeout = 20 * time.Second

func googleCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, googleTimeout)
}

// Tick polls every due huddle, ends finished ones, expires rings and runs due follow-ups.
// Called every 2 s; all state lives in the KV, so it resumes after a plugin restart.
// Errors are logged per huddle and never stop the others.
// ponytail: one full ListKeys scan per tick; TTLs keep the keyspace small, add a due-index if it grows.
func (s *Service) Tick(ctx context.Context) {
	now := s.now()
	hs, rings, fs, err := s.store.TickState()
	s.logLoadErr(err)
	for _, h := range hs { // ponytail: sequential polls; errgroup if many huddles run at once
		if h.NextPollAt.After(now) {
			continue
		}
		if err := s.poll(ctx, h, now); err != nil && !errors.Is(err, ErrEnded) {
			s.mm.LogError("huddle: tick: poll failed", "huddle_id", h.ID, "err", err.Error())
		}
	}
	s.ringer.expire(ctx, now, rings)
	s.followups(ctx, now, fs)
}

// logLoadErr logs a TickState error only when it differs from the last one, so an unreadable
// entry is not logged every 2 s forever.
func (s *Service) logLoadErr(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if last, _ := s.lastLoadErr.Swap(msg).(string); err != nil && msg != last {
		s.mm.LogError("huddle: tick: loading state failed", "err", msg)
	}
}

// poll fetches the Meet state for h, merges it and ends the huddle if it is over.
// Google and MM lookups happen before the CAS update, whose closure may run several times;
// side effects follow from the last attempt's result.
func (s *Service) poll(ctx context.Context, h *Huddle, now time.Time) error {
	var (
		conference      string
		gps             []google.Participant
		gerr            error
		resolved, names map[string]string
	)
	// A decided or overdue end needs no Meet state; the update below still persists it.
	if !h.Finishing && now.Sub(h.StartedAt) <= maxHuddle {
		gctx, cancel := googleCtx(ctx)
		if conference, gps, gerr = s.fetch(gctx, h); gerr != nil {
			s.mm.LogError("huddle: tick: google failed", "huddle_id", h.ID, "err", gerr.Error())
		} else {
			resolved, names = s.resolve(gctx, h, gps)
		}
		cancel()
	}

	// A requested end is sent to Meet (and retried) every poll; the huddle ends at finishBound anyway.
	if !h.EndingSince.IsZero() && !h.Finishing && conference != "" {
		gctx, cancel := googleCtx(ctx)
		if err := s.meet.EndActiveConference(gctx, h.OwnerEmail, h.Space); err != nil {
			s.mm.LogError("huddle: tick: ending the conference failed", "huddle_id", h.ID, "err", err.Error())
		}
		cancel()
	}

	var (
		r            MergeResult
		end, changed bool
	)
	fresh, err := s.update(h, func(x *Huddle) error {
		r, changed = MergeResult{}, false
		before := view(x)
		end = decideEnd(x, gerr == nil && conference == "", now)
		x.NextPollAt = now.Add(pollSlow)
		// Fast while waiting out endGrace; not while finishing or while Google fails.
		if now.Before(x.BoostUntil) || !end && gerr == nil && !x.GoneSince.IsZero() {
			x.NextPollAt = now.Add(pollFast)
		}
		if end {
			// Persisted so a failed finish is retried as is and joins/invites are refused meanwhile.
			x.Finishing = true
			if x.EndingSince.IsZero() {
				x.EndingSince = now
			}
			return nil
		}
		if gerr != nil || conference == "" {
			return nil
		}
		if !slices.Contains(x.ConferenceRecords, conference) { // a rollover adds one; follow-ups cover all
			x.ConferenceRecords = append(x.ConferenceRecords, conference)
		}
		x.GoneSince = time.Time{}
		r = Merge(x, gps, func(gid string) (string, bool) { uid, ok := resolved[gid]; return uid, ok }, names, now)
		// After Merge, "no confirmed participant without MissingSince" is exactly "Meet lists nobody".
		x.Ending = len(gps) == 0
		changed = !reflect.DeepEqual(before, view(x))
		return nil
	})
	if err != nil {
		return err
	}
	if end {
		gctx, cancel := googleCtx(ctx)
		defer cancel()
		return s.finish(gctx, fresh, now)
	}
	for _, p := range r.Joined {
		if p.UserID != "" {
			s.presence.joined(p.UserID, fresh)
		}
	}
	for _, p := range r.Left {
		if p.UserID != "" {
			s.presence.left(p.UserID, fresh)
		}
	}
	if changed {
		s.updateCard(fresh)
		s.publish(view(fresh))
	}
	return nil
}

// decideEnd decides the end of x; gone: Meet answered and has no active conference. It records
// GoneSince when a seen conference disappears, which ends the huddle only after endGrace.
func decideEnd(x *Huddle, gone bool, now time.Time) bool {
	ending := !x.EndingSince.IsZero()
	// The Meet call outlived the end request for finishBound: end the huddle without it.
	stuck := ending && now.Sub(x.EndingSince) > finishBound
	if x.Finishing || now.Sub(x.StartedAt) > maxHuddle || stuck {
		return true
	}
	switch {
	case !gone:
		return false
	case ending:
		return true
	case len(x.ConferenceRecords) == 0:
		return now.Sub(x.StartedAt) > noConference
	case x.GoneSince.IsZero():
		x.GoneSince = now
	}
	return now.Sub(x.GoneSince) >= endGrace
}

// fetch returns the active conference record ("" if none) and its participants. A deleted
// space (404) counts as no conference; an unconfigured Google client is an error like any
// other (the key may come back), so it never ends a huddle by itself.
func (s *Service) fetch(ctx context.Context, h *Huddle) (string, []google.Participant, error) {
	space, err := s.meet.GetSpace(ctx, h.OwnerEmail, h.Space)
	if google.IsNotFound(err) {
		s.mm.LogError("huddle: tick: Meet space unavailable, treating the conference as over", "huddle_id", h.ID, "err", err.Error())
		return "", nil, nil
	}
	if err != nil || space.ActiveConference == "" {
		return "", nil, err
	}
	gps, err := s.meet.ActiveParticipants(ctx, h.OwnerEmail, space.ActiveConference)
	return space.ActiveConference, gps, err
}

// resolve maps the signed-in Google users in gps to MM users (googleUserID → userID) and
// returns display names for resolved users not yet in the huddle.
func (s *Service) resolve(ctx context.Context, h *Huddle, gps []google.Participant) (resolved, names map[string]string) {
	resolved, names = map[string]string{}, map[string]string{}
	for _, g := range gps {
		if g.GoogleUserID == "" {
			continue
		}
		uid := s.lookupGoogle(ctx, h, g.GoogleUserID)
		if uid == "" {
			continue
		}
		resolved[g.GoogleUserID] = uid
		if _, known := h.Participants[uid]; !known {
			if u, appErr := s.mm.GetUser(uid); appErr == nil {
				names[uid] = displayName(u)
			}
		}
	}
	return resolved, names
}

// lookupGoogle resolves one Google user: gid: cache, else the Directory (as the owner) plus a
// reverse lookup in GoogleAccounts. Misses (outside the domain or unmapped) are cached in gx:
// for missTTL; errors are not cached.
func (s *Service) lookupGoogle(ctx context.Context, h *Huddle, gid string) string {
	uid, err := s.store.GoogleMapping(gid)
	if err != nil {
		s.mm.LogError("huddle: tick: reading google mapping failed", "google_user_id", gid, "err", err.Error())
		return ""
	}
	if uid != "" {
		return uid
	}
	missed, err := s.store.GoogleMiss(gid)
	if err != nil {
		s.mm.LogError("huddle: tick: reading google miss failed", "google_user_id", gid, "err", err.Error())
		return ""
	}
	if missed {
		return ""
	}
	email, found, err := s.meet.LookupEmail(ctx, h.OwnerEmail, gid)
	if err != nil {
		s.mm.LogError("huddle: tick: directory lookup failed", "huddle_id", h.ID, "google_user_id", gid, "err", err.Error())
		return ""
	}
	if found {
		for u, e := range s.cfg().GoogleAccounts {
			if strings.EqualFold(e, email) {
				uid = u
				break
			}
		}
	}
	if uid == "" {
		err = s.store.SaveGoogleMiss(gid)
	} else {
		err = s.store.SaveGoogleMapping(gid, uid)
	}
	if err != nil {
		s.mm.LogError("huddle: tick: caching google lookup failed", "google_user_id", gid, "err", err.Error())
	}
	return uid
}

// finish ends h. Every step is safe to repeat: if one fails, the huddle stays active
// (Finishing) and the next tick retries all of them. EndHuddle comes last. Once finishBound
// has passed since EndingSince, Meet and follow-up failures are only logged so the huddle
// always ends; only a KV failure in EndHuddle keeps it. Open rings of h are closed at the end.
// ponytail: one GetUser per seen user; batch with GetUsersByIds if huddles get large.
func (s *Service) finish(ctx context.Context, h *Huddle, now time.Time) error {
	var names []string
	for _, id := range h.SeenUserIDs {
		if u, appErr := s.mm.GetUser(id); appErr == nil {
			names = append(names, displayName(u))
		}
	}
	s.editCard(h, buildSummaryPost(h, names, now.Sub(h.StartedAt)))
	for _, p := range h.Participants {
		if p.UserID != "" {
			s.presence.left(p.UserID, h)
		}
	}
	giveUp := now.Sub(h.EndingSince) > finishBound
	if err := s.meet.RestrictSpace(ctx, h.OwnerEmail, h.Space); err != nil {
		if !giveUp && !google.IsNotFound(err) {
			return fmt.Errorf("restrict space: %w", err)
		}
		s.mm.LogError("huddle: tick: restricting Meet space failed, ending anyway; the space stays open", "huddle_id", h.ID, "space", h.Space, "err", err.Error())
	}
	if len(h.ConferenceRecords) > 0 && h.PostID != "" { // replies need the card as root
		f, err := s.store.Followup(h.ID)
		if err == nil && f == nil { // a retry must not reset Posted
			err = s.store.SaveFollowup(&Followup{
				HuddleID: h.ID, ChannelID: h.ChannelID, PostID: h.PostID, OwnerEmail: h.OwnerEmail, ConferenceRecords: h.ConferenceRecords,
				Locale: h.StarterLocale, Until: now.Add(followupFor), NextAt: now.Add(followupEvery),
			})
		}
		if err != nil {
			if !giveUp {
				return fmt.Errorf("follow-up: %w", err)
			}
			s.mm.LogError("huddle: tick: saving follow-up failed, ending without it", "huddle_id", h.ID, "err", err.Error())
		}
	}
	if err := s.store.EndHuddle(h); err != nil {
		return err
	}
	v := view(h)
	v.Ended = true
	s.publish(v)
	s.closeRings(h.ID)
	return nil
}

// closeRings closes the dialogs of h's open rings. The rings stay ringing: expire turns them into
// missed calls (bot DM update, caller toast); a late accept is refused by running. Best effort.
func (s *Service) closeRings(huddleID string) {
	rings, err := s.store.Rings()
	if err != nil {
		s.mm.LogError("huddle: tick: listing rings failed", "huddle_id", huddleID, "err", err.Error())
	}
	for _, ring := range rings {
		if ring.HuddleID != huddleID || ring.State != ringRinging {
			continue
		}
		publishRingEnded(s.mm, ring)
	}
}

// followups posts new conference artifacts as bot replies under the card, every 5 min for 2 h after the end.
func (s *Service) followups(ctx context.Context, now time.Time, fs []*Followup) {
	for _, f := range fs {
		if now.After(f.Until) {
			s.deleteFollowup(f)
			continue
		}
		if f.NextAt.After(now) {
			continue
		}
		f.NextAt = now.Add(followupEvery)
		var arts []google.Artifact
		for _, rec := range f.ConferenceRecords {
			gctx, cancel := googleCtx(ctx)
			a, err := s.meet.Artifacts(gctx, f.OwnerEmail, rec)
			cancel()
			if err != nil {
				s.mm.LogError("huddle: tick: loading artifacts failed", "huddle_id", f.HuddleID, "conference", rec, "err", err.Error())
			}
			arts = append(arts, a...)
		}
		if s.postArtifacts(f, arts) {
			s.saveFollowup(f)
		}
	}
}

// postArtifacts replies with every artifact not posted yet, saving f after each one so a
// later failure never duplicates it. Returns false if the card is gone and f was deleted.
func (s *Service) postArtifacts(f *Followup, arts []google.Artifact) bool {
	for _, a := range arts {
		if slices.Contains(f.Posted, a.Name) {
			continue
		}
		reply := &model.Post{UserId: s.cfg().BotUserID, ChannelId: f.ChannelID, RootId: f.PostID, Message: fmt.Sprintf(Text(f.Locale, a.Kind), a.URL)}
		if _, appErr := s.mm.CreatePost(reply); appErr != nil {
			s.mm.LogError("huddle: tick: posting artifact failed", "huddle_id", f.HuddleID, "artifact", a.Name, "err", appErr.Error())
			if appErr.StatusCode >= 400 && appErr.StatusCode < 500 { // root post deleted: retrying won't help
				s.deleteFollowup(f)
				return false
			}
			continue
		}
		f.Posted = append(f.Posted, a.Name)
		s.saveFollowup(f)
	}
	return true
}

func (s *Service) saveFollowup(f *Followup) {
	if err := s.store.SaveFollowup(f); err != nil {
		s.mm.LogError("huddle: tick: saving follow-up failed", "huddle_id", f.HuddleID, "err", err.Error())
	}
}

func (s *Service) deleteFollowup(f *Followup) {
	if err := s.store.DeleteFollowup(f.HuddleID); err != nil {
		s.mm.LogError("huddle: tick: deleting follow-up failed", "huddle_id", f.HuddleID, "err", err.Error())
	}
}
