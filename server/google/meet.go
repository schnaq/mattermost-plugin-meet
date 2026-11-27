// Package google wraps the Meet REST v2 and Admin Directory APIs using
// domain-wide delegation (a service account impersonating the owner).
package google

import "context"

// Space is a Meet space. ActiveConference is the conference record name, "" if none.
type Space struct{ Name, MeetingURI, ActiveConference string }

// Participant is a current or past conference participant. GoogleUserID is
// "users/123", empty for anonymous and phone participants.
type Participant struct {
	GoogleUserID, DisplayName string
	Anonymous                 bool
}

// Artifact is a conference artifact. Kind: "recording" | "transcript" | "smart_notes".
type Artifact struct{ Kind, Name, URL string }

// Meet is the Google surface the plugin needs. owner is the impersonated Google email.
type Meet interface {
	CreateSpace(ctx context.Context, owner string) (Space, error)
	GetSpace(ctx context.Context, owner, space string) (Space, error)
	ActiveParticipants(ctx context.Context, owner, conferenceRecord string) ([]Participant, error)
	// EndActiveConference returns nil when no conference is active.
	EndActiveConference(ctx context.Context, owner, space string) error
	RestrictSpace(ctx context.Context, owner, space string) error
	Artifacts(ctx context.Context, owner, conferenceRecord string) ([]Artifact, error)
	// LookupEmail: users outside the domain (404/403) yield found=false, err=nil.
	LookupEmail(ctx context.Context, owner, googleUserID string) (email string, found bool, err error)
}
