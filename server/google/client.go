package google

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"
	meetapi "google.golang.org/api/meet/v2"
	"google.golang.org/api/option"
)

const (
	scopeMeet  = "https://www.googleapis.com/auth/meetings.space.created"
	scopeAdmin = "https://www.googleapis.com/auth/admin.directory.user.readonly"

	noActiveConference = "There is no active conference for the given space."

	// httpTimeout bounds every Google HTTP request, token exchanges included.
	httpTimeout = 30 * time.Second
)

type services struct {
	meet  *meetapi.Service
	admin *admin.Service
}

type client struct {
	key  []byte
	opts []option.ClientOption

	mu    sync.Mutex
	cache map[string]*services // per owner
}

// New parses the service-account key once; services are built lazily per
// owner (Subject = owner) and cached. Extra opts are applied last, so
// option.WithHTTPClient/WithEndpoint override the JWT transport (used by tests,
// which may pass a dummy key: the key is only parsed, never used to sign then).
func New(serviceAccountJSON []byte, opts ...option.ClientOption) (Meet, error) {
	if _, err := google.JWTConfigFromJSON(serviceAccountJSON, scopeMeet, scopeAdmin); err != nil {
		return nil, err
	}
	return &client{key: serviceAccountJSON, opts: opts, cache: map[string]*services{}}, nil
}

func (c *client) svc(ctx context.Context, owner string) (*services, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.cache[owner]; ok {
		return s, nil
	}
	jc, err := google.JWTConfigFromJSON(c.key, scopeMeet, scopeAdmin)
	if err != nil {
		return nil, err
	}
	jc.Subject = owner
	// The client outlives the request ctx, so it gets a background one; its value gives the
	// token exchange a timeout too. WithHTTPClient wins over other auth options, so the client
	// carries the token source itself; test opts come later and replace it.
	hc := jc.Client(context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: httpTimeout}))
	hc.Timeout = httpTimeout
	opts := append([]option.ClientOption{option.WithHTTPClient(hc)}, c.opts...)
	m, err := meetapi.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	a, err := admin.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	s := &services{meet: m, admin: a}
	c.cache[owner] = s
	return s, nil
}

// IsNotFound reports whether err is a Google API 404 (e.g. a deleted space).
func IsNotFound(err error) bool {
	var ge *googleapi.Error
	return errors.As(err, &ge) && ge.Code == http.StatusNotFound
}

func toSpace(s *meetapi.Space) Space {
	out := Space{Name: s.Name, MeetingURI: s.MeetingUri}
	if s.ActiveConference != nil {
		out.ActiveConference = s.ActiveConference.ConferenceRecord
	}
	return out
}

func (c *client) CreateSpace(ctx context.Context, owner string) (Space, error) {
	s, err := c.svc(ctx, owner)
	if err != nil {
		return Space{}, err
	}
	sp, err := s.meet.Spaces.Create(&meetapi.Space{Config: &meetapi.SpaceConfig{AccessType: "OPEN"}}).Context(ctx).Do()
	if err != nil {
		return Space{}, err
	}
	return toSpace(sp), nil
}

func (c *client) GetSpace(ctx context.Context, owner, space string) (Space, error) {
	s, err := c.svc(ctx, owner)
	if err != nil {
		return Space{}, err
	}
	sp, err := s.meet.Spaces.Get(space).Context(ctx).Do()
	if err != nil {
		return Space{}, err
	}
	return toSpace(sp), nil
}

func (c *client) ActiveParticipants(ctx context.Context, owner, conferenceRecord string) ([]Participant, error) {
	s, err := c.svc(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []Participant
	err = s.meet.ConferenceRecords.Participants.List(conferenceRecord).Filter("latest_end_time IS NULL").
		Context(ctx).Pages(ctx, func(r *meetapi.ListParticipantsResponse) error {
		for _, p := range r.Participants {
			switch {
			case p.SignedinUser != nil:
				out = append(out, Participant{GoogleUserID: p.SignedinUser.User, DisplayName: p.SignedinUser.DisplayName})
			case p.AnonymousUser != nil:
				out = append(out, Participant{DisplayName: p.AnonymousUser.DisplayName, Anonymous: true})
			case p.PhoneUser != nil:
				out = append(out, Participant{DisplayName: p.PhoneUser.DisplayName, Anonymous: true})
			}
		}
		return nil
	})
	return out, err
}

func (c *client) EndActiveConference(ctx context.Context, owner, space string) error {
	s, err := c.svc(ctx, owner)
	if err != nil {
		return err
	}
	_, err = s.meet.Spaces.EndActiveConference(space, &meetapi.EndActiveConferenceRequest{}).Context(ctx).Do()
	var ge *googleapi.Error
	if errors.As(err, &ge) && ge.Code == http.StatusBadRequest && strings.Contains(ge.Message, noActiveConference) {
		return nil
	}
	return err
}

func (c *client) RestrictSpace(ctx context.Context, owner, space string) error {
	s, err := c.svc(ctx, owner)
	if err != nil {
		return err
	}
	_, err = s.meet.Spaces.Patch(space, &meetapi.Space{Config: &meetapi.SpaceConfig{AccessType: "RESTRICTED"}}).
		UpdateMask("config.accessType").Context(ctx).Do()
	return err
}

func (c *client) Artifacts(ctx context.Context, owner, conferenceRecord string) ([]Artifact, error) {
	s, err := c.svc(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []Artifact
	add := func(kind, name, url string) {
		if url != "" {
			out = append(out, Artifact{Kind: kind, Name: name, URL: url})
		}
	}
	err = s.meet.ConferenceRecords.Recordings.List(conferenceRecord).Context(ctx).Pages(ctx, func(r *meetapi.ListRecordingsResponse) error {
		for _, x := range r.Recordings {
			if x.DriveDestination != nil {
				add("recording", x.Name, x.DriveDestination.ExportUri)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = s.meet.ConferenceRecords.Transcripts.List(conferenceRecord).Context(ctx).Pages(ctx, func(r *meetapi.ListTranscriptsResponse) error {
		for _, x := range r.Transcripts {
			if x.DocsDestination != nil {
				add("transcript", x.Name, x.DocsDestination.ExportUri)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = s.meet.ConferenceRecords.SmartNotes.List(conferenceRecord).Context(ctx).Pages(ctx, func(r *meetapi.ListSmartNotesResponse) error {
		for _, x := range r.SmartNotes {
			if x.DocsDestination != nil {
				add("smart_notes", x.Name, x.DocsDestination.ExportUri)
			}
		}
		return nil
	})
	return out, err
}

func (c *client) LookupEmail(ctx context.Context, owner, googleUserID string) (string, bool, error) {
	s, err := c.svc(ctx, owner)
	if err != nil {
		return "", false, err
	}
	u, err := s.admin.Users.Get(strings.TrimPrefix(googleUserID, "users/")).ViewType("domain_public").Context(ctx).Do()
	var ge *googleapi.Error
	if errors.As(err, &ge) && (ge.Code == http.StatusNotFound || ge.Code == http.StatusForbidden) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return u.PrimaryEmail, true, nil
}
