package google

import (
	"context"
	"errors"
	"sync"
)

// ErrNotConfigured is returned while no valid service account key is loaded.
var ErrNotConfigured = errors.New("google service account is not configured")

// Holder is a Meet whose client can be swapped at runtime (configuration changes).
// The zero value is usable and reports ErrNotConfigured.
type Holder struct {
	mu sync.RWMutex
	m  Meet
}

// Set replaces the client; nil unconfigures.
func (h *Holder) Set(m Meet) {
	h.mu.Lock()
	h.m = m
	h.mu.Unlock()
}

func (h *Holder) get() (Meet, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.m == nil {
		return nil, ErrNotConfigured
	}
	return h.m, nil
}

func (h *Holder) CreateSpace(ctx context.Context, owner string) (Space, error) {
	m, err := h.get()
	if err != nil {
		return Space{}, err
	}
	return m.CreateSpace(ctx, owner)
}

func (h *Holder) GetSpace(ctx context.Context, owner, space string) (Space, error) {
	m, err := h.get()
	if err != nil {
		return Space{}, err
	}
	return m.GetSpace(ctx, owner, space)
}

func (h *Holder) ActiveParticipants(ctx context.Context, owner, conferenceRecord string) ([]Participant, error) {
	m, err := h.get()
	if err != nil {
		return nil, err
	}
	return m.ActiveParticipants(ctx, owner, conferenceRecord)
}

func (h *Holder) EndActiveConference(ctx context.Context, owner, space string) error {
	m, err := h.get()
	if err != nil {
		return err
	}
	return m.EndActiveConference(ctx, owner, space)
}

func (h *Holder) RestrictSpace(ctx context.Context, owner, space string) error {
	m, err := h.get()
	if err != nil {
		return err
	}
	return m.RestrictSpace(ctx, owner, space)
}

func (h *Holder) Artifacts(ctx context.Context, owner, conferenceRecord string) ([]Artifact, error) {
	m, err := h.get()
	if err != nil {
		return nil, err
	}
	return m.Artifacts(ctx, owner, conferenceRecord)
}

func (h *Holder) LookupEmail(ctx context.Context, owner, googleUserID string) (string, bool, error) {
	m, err := h.get()
	if err != nil {
		return "", false, err
	}
	return m.LookupEmail(ctx, owner, googleUserID)
}
