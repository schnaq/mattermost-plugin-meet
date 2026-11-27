// Package huddle holds the huddle model and its KV-backed store.
package huddle

import (
	"errors"
	"fmt"
	"time"
)

type ParticipantState string

const (
	StateJoining   ParticipantState = "joining"
	StateConfirmed ParticipantState = "confirmed"
)

type Participant struct {
	UserID            string
	DisplayName       string
	GoogleUserID      string
	GoogleDisplayName string // name shown in Meet, to re-identify correlated anonymous users
	State             ParticipantState
	Since             time.Time
	MissingSince      time.Time // first tick a confirmed participant was absent from Meet; zero while present
}

type Huddle struct {
	ID            string
	ChannelID     string
	Space         string
	MeetingURI    string
	OwnerEmail    string
	StarterUserID string
	// StarterLocale renders the card, summary and follow-ups (one post serves every member).
	StarterLocale string
	PostID        string
	// ConferenceRecords: every Meet conference seen in the space, oldest first (a reload or drop can
	// roll over to a new one); never cleared.
	ConferenceRecords []string
	// GoneSince: first poll that found the seen conference gone; zero while it runs. The huddle
	// ends once it stays gone for endGrace.
	GoneSince  time.Time
	StartedAt  time.Time
	BoostUntil time.Time
	NextPollAt time.Time
	// Ending: conference active with 0 participants (tick).
	Ending bool
	// EndingSince: when the end was requested (starter's "end for all") or decided (tick); zero while
	// running. First setter wins; no more joins/invites once set. Bounds how long Google failures may
	// delay the end (finishBound).
	// ponytail: EndingSince/Finishing are a time and a bool, not a phase enum; fold them if a third phase appears.
	EndingSince time.Time
	// Finishing: the tick decided the end; finish is retried until done.
	Finishing bool
	// RingSettled: a ring was accepted or the missed hint posted; no further hint.
	RingSettled bool
	// SeenUserIDs: every MM user ever confirmed, for the summary.
	SeenUserIDs []string
	// Participants key: UserID, else "g:"+GoogleUserID, else "a:"+DisplayName.
	Participants map[string]Participant
}

// Ring State: ringing|accepted|declined|missed.
type Ring struct {
	ID, HuddleID, ChannelID, From, To, PostID, State string
	Targeted                                         bool
	CreatedAt                                        time.Time
}

type Followup struct {
	HuddleID, ChannelID, PostID, OwnerEmail string
	ConferenceRecords                       []string
	Locale                                  string // starter's, for the replies
	Until, NextAt                           time.Time
	Posted                                  []string // artifact names already posted
}

type HuddleView struct {
	ID           string            `json:"id"`
	ChannelID    string            `json:"channel_id"`
	StarterID    string            `json:"starter_id"`
	StartedAt    int64             `json:"started_at"`
	Ending       bool              `json:"ending"`        // conference active but empty; still joinable
	EndRequested bool              `json:"end_requested"` // EndingSince set: joins and invites are refused
	Ended        bool              `json:"ended"`
	Participants []ParticipantView `json:"participants"`
}

type ParticipantView struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	State       string `json:"state"`
}

var (
	ErrForbidden   = errors.New("forbidden")
	ErrEnded       = errors.New("huddle ended")
	ErrNoOwner     = errors.New("no owner")
	ErrRateLimited = errors.New("rate limited")
	ErrNotFound    = errors.New("not found")
	// ErrBadRequest marks a malformed client request; specific causes wrap it.
	ErrBadRequest = errors.New("bad request")
	ErrNotMember  = fmt.Errorf("invite target is not a channel member: %w", ErrBadRequest)
	// ErrMeetUnavailable: a Google call failed; details are only logged, never returned.
	ErrMeetUnavailable = errors.New("google meet is unavailable, please try again later")
)
