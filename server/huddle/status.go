package huddle

import (
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// savedStatus is always stored as a struct: SaveStatus(uid, nil) would delete the key,
// which would be indistinguishable from "never joined".
type savedStatus struct {
	Had    bool
	Status *model.CustomStatus
	// Applied is the huddle status we set; left restores only while it is still in place.
	Applied *model.CustomStatus
}

type statusPresence struct {
	mm    MM
	store Store
	cfg   func() Config
}

func newStatusPresence(mm MM, store Store, cfg func() Config) presence {
	return &statusPresence{mm, store, cfg}
}

// inOther reports whether the user is a confirmed participant in an active huddle other than h.
func (s *statusPresence) inOther(userID string, h *Huddle) bool {
	all, _, err := s.store.ActiveHuddles() // unreadable entries can't own the status
	if err != nil {
		s.mm.LogError("huddle status: list huddles", "err", err.Error())
		return false
	}
	for _, o := range all {
		if o.ID != h.ID && o.Participants[userID].State == StateConfirmed {
			return true
		}
	}
	return false
}

func (s *statusPresence) joined(userID string, h *Huddle) {
	found, err := s.store.SavedStatus(userID, &savedStatus{})
	if err != nil {
		s.mm.LogError("huddle status: read saved", "err", err.Error())
		return
	}
	if found || s.inOther(userID, h) {
		return // already ours, or another huddle owns the status
	}
	u, appErr := s.mm.GetUser(userID)
	if appErr != nil {
		s.mm.LogError("huddle status: get user", "err", appErr.Error())
		return
	}
	// Save first: without a saved status we could never restore. If setting then fails,
	// drop the saved copy so a later left cannot "restore" over what the user has now.
	cur := u.GetCustomStatus()
	c := s.cfg()
	applied := &model.CustomStatus{Emoji: c.StatusEmoji, Text: c.StatusText}
	applied.PreSave() // as the server stores it (text truncated), so left compares like with like
	if err := s.store.SaveStatus(userID, savedStatus{Had: cur != nil, Status: cur, Applied: applied}); err != nil {
		s.mm.LogError("huddle status: save", "err", err.Error())
		return
	}
	if appErr := s.mm.UpdateUserCustomStatus(userID, applied); appErr != nil {
		s.mm.LogError("huddle status: set", "err", appErr.Error())
		_ = s.store.DeleteStatus(userID)
	}
}

// left is idempotent: no saved status means nothing to do.
func (s *statusPresence) left(userID string, h *Huddle) {
	var saved savedStatus
	found, err := s.store.SavedStatus(userID, &saved)
	if err != nil {
		s.mm.LogError("huddle status: read saved", "err", err.Error())
		return
	}
	if !found || s.inOther(userID, h) {
		return
	}
	// On MM errors the saved copy stays, so the finish retry can try again; the "still ours" check below keeps a late retry harmless.
	u, appErr := s.mm.GetUser(userID)
	if appErr != nil {
		s.mm.LogError("huddle status: get user", "err", appErr.Error())
		return
	}
	cur, a := u.GetCustomStatus(), saved.Applied
	if cur != nil && a != nil && cur.Emoji == a.Emoji && cur.Text == a.Text {
		s.restore(userID, saved)
		return
	}
	s.drop(userID) // user changed it meanwhile (or cleared it): not ours anymore
}

func (s *statusPresence) drop(userID string) {
	if err := s.store.DeleteStatus(userID); err != nil {
		s.mm.LogError("huddle status: delete saved", "err", err.Error())
	}
}

func (s *statusPresence) restore(userID string, saved savedStatus) {
	var appErr *model.AppError
	if !saved.Had || saved.Status == nil || (!saved.Status.ExpiresAt.IsZero() && saved.Status.ExpiresAt.Before(time.Now())) {
		appErr = s.mm.RemoveUserCustomStatus(userID)
	} else {
		appErr = s.mm.UpdateUserCustomStatus(userID, saved.Status)
	}
	if appErr != nil {
		s.mm.LogError("huddle status: restore", "err", appErr.Error())
		return
	}
	s.drop(userID)
}
