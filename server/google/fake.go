package google

import (
	"context"
	"sync"
)

// Fake is an in-memory Meet for tests. Preset the exported fields, then read
// Calls ("Method owner arg") afterwards. Err[method] is returned by that method.
type Fake struct {
	mu sync.Mutex

	Space        Space             // returned by CreateSpace/GetSpace
	Participants []Participant     // returned by ActiveParticipants
	Emails       map[string]string // googleUserID -> email; missing = not in domain
	ArtifactList []Artifact        // returned by Artifacts
	Err          map[string]error  // keyed by method name
	Calls        []string
}

var _ Meet = (*Fake)(nil)

func (f *Fake) rec(method, owner, arg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, method+" "+owner+" "+arg)
	return f.Err[method]
}

func (f *Fake) CreateSpace(_ context.Context, owner string) (Space, error) {
	return f.Space, f.rec("CreateSpace", owner, "")
}

func (f *Fake) GetSpace(_ context.Context, owner, space string) (Space, error) {
	return f.Space, f.rec("GetSpace", owner, space)
}

func (f *Fake) ActiveParticipants(_ context.Context, owner, rec string) ([]Participant, error) {
	return f.Participants, f.rec("ActiveParticipants", owner, rec)
}

func (f *Fake) EndActiveConference(_ context.Context, owner, space string) error {
	return f.rec("EndActiveConference", owner, space)
}

func (f *Fake) RestrictSpace(_ context.Context, owner, space string) error {
	return f.rec("RestrictSpace", owner, space)
}

func (f *Fake) Artifacts(_ context.Context, owner, rec string) ([]Artifact, error) {
	return f.ArtifactList, f.rec("Artifacts", owner, rec)
}

func (f *Fake) LookupEmail(_ context.Context, owner, id string) (string, bool, error) {
	err := f.rec("LookupEmail", owner, id)
	email, ok := f.Emails[id]
	return email, ok, err
}
