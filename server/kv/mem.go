package kv

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"
)

type entry struct {
	b   []byte
	exp time.Time // zero: never expires
}

type mem struct {
	mu  sync.Mutex
	m   map[string]entry
	now func() time.Time
}

// NewMem returns an in-memory KV for tests, safe for concurrent use.
func NewMem() KV { return NewMemClock(time.Now) }

// NewMemClock is NewMem with a clock for TTL expiry.
func NewMemClock(now func() time.Time) KV { return &mem{m: map[string]entry{}, now: now} }

// get returns the live value; the caller holds mu.
func (s *mem) get(key string) ([]byte, bool) {
	e, ok := s.m[key]
	if !ok || !e.exp.IsZero() && !s.now().Before(e.exp) {
		return nil, false
	}
	return e.b, true
}

// put stores b (nil deletes); the caller holds mu.
func (s *mem) put(key string, b []byte, ttl time.Duration) {
	if b == nil {
		delete(s.m, key)
		return
	}
	e := entry{b: b}
	if ttl > 0 {
		e.exp = s.now().Add(ttl)
	}
	s.m[key] = e
}

func (s *mem) Get(key string, out any) (bool, error) {
	s.mu.Lock()
	b, ok := s.get(key)
	s.mu.Unlock()
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(b, out)
}

func (s *mem) Set(key string, v any) error { return s.SetWithTTL(key, v, 0) }

func (s *mem) SetWithTTL(key string, v any, ttl time.Duration) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.put(key, b, ttl)
	return nil
}

func (s *mem) CompareAndSet(key string, old, new any) (bool, error) {
	ob, err := encode(old)
	if err != nil {
		return false, err
	}
	nb, err := encode(new)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, present := s.get(key)
	if (ob == nil && present) || (ob != nil && (!present || !bytes.Equal(cur, ob))) {
		return false, nil
	}
	s.put(key, nb, 0)
	return true, nil
}

func (s *mem) Delete(key string) error { return s.Set(key, nil) }

func (s *mem) ListKeys(prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.m {
		if _, live := s.get(k); live && strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out, nil
}
