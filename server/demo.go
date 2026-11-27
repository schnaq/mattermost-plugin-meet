//go:build demo

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
)

// Demo build for screenshots and local development without Google Workspace:
// `make dist GO_BUILD_FLAGS='-tags demo'`, any non-empty ServiceAccountJSON.
// Every huddle has an active conference until it is ended; the emails in MEET_DEMO_PARTICIPANTS
// (comma-separated, server environment) are shown as joined.
func init() {
	newMeet = func([]byte) (google.Meet, error) { return demoMeet{}, nil }
}

type demoMeet struct{}

var (
	demoSeq   atomic.Int64
	demoEnded sync.Map // space name -> struct{}
)

func demoSpace(name string) google.Space {
	sp := google.Space{Name: name, MeetingURI: "https://meet.google.com/abc-mnop-xyz", ActiveConference: name + "/conference"}
	if _, ended := demoEnded.Load(name); ended {
		sp.ActiveConference = ""
	}
	return sp
}

func (demoMeet) CreateSpace(context.Context, string) (google.Space, error) {
	return demoSpace(fmt.Sprintf("spaces/demo-%d", demoSeq.Add(1))), nil
}

func (demoMeet) GetSpace(_ context.Context, _, space string) (google.Space, error) {
	return demoSpace(space), nil
}

func (demoMeet) EndActiveConference(_ context.Context, _, space string) error {
	demoEnded.Store(space, struct{}{})
	return nil
}
func (demoMeet) RestrictSpace(context.Context, string, string) error { return nil }
func (demoMeet) Artifacts(context.Context, string, string) ([]google.Artifact, error) {
	return nil, nil
}

func (demoMeet) ActiveParticipants(context.Context, string, string) ([]google.Participant, error) {
	var ps []google.Participant
	for _, e := range strings.Split(os.Getenv("MEET_DEMO_PARTICIPANTS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			ps = append(ps, google.Participant{GoogleUserID: "users/" + e, DisplayName: e})
		}
	}
	return ps, nil
}

func (demoMeet) LookupEmail(_ context.Context, _, id string) (string, bool, error) {
	return strings.TrimPrefix(id, "users/"), true, nil
}
