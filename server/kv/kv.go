// Package kv is a small key-value abstraction over the Mattermost plugin KV store.
package kv

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// KV stores JSON values. CompareAndSet compares the JSON encoding of old
// (json.RawMessage and []byte are used as-is) with the stored bytes.
// old==nil: only if absent; new==nil: delete.
type KV interface {
	Get(key string, out any) (found bool, err error)
	Set(key string, v any) error
	// SetWithTTL is Set with expiry: after ttl the key reads as absent (whole seconds; 0: never).
	SetWithTTL(key string, v any, ttl time.Duration) error
	CompareAndSet(key string, old, new any) (ok bool, err error)
	Delete(key string) error
	ListKeys(prefix string) ([]string, error)
}

// encode returns the JSON bytes for v; nil stays nil.
func encode(v any) ([]byte, error) {
	switch b := v.(type) {
	case nil:
		return nil, nil
	case []byte:
		return b, nil
	case json.RawMessage:
		return b, nil
	}
	return json.Marshal(v)
}

type pluginKV struct{ c *pluginapi.Client }

// New wraps the plugin API client.
func New(client *pluginapi.Client) KV { return pluginKV{client} }

func (p pluginKV) Get(key string, out any) (bool, error) {
	var raw []byte
	if err := p.c.KV.Get(key, &raw); err != nil {
		return false, err
	}
	if len(raw) == 0 {
		return false, nil
	}
	return true, json.Unmarshal(raw, out)
}

func (p pluginKV) Set(key string, v any) error { return p.SetWithTTL(key, v, 0) }

func (p pluginKV) SetWithTTL(key string, v any, ttl time.Duration) error {
	b, err := encode(v)
	if err != nil {
		return err
	}
	_, err = p.c.KV.Set(key, b, pluginapi.SetExpiry(ttl))
	return err
}

func (p pluginKV) CompareAndSet(key string, old, new any) (bool, error) {
	ob, err := encode(old)
	if err != nil {
		return false, err
	}
	nb, err := encode(new)
	if err != nil {
		return false, err
	}
	// pluginapi treats a nil oldValue as "only if absent"; a typed nil []byte is not nil.
	var o any
	if ob != nil {
		o = ob
	}
	var n any
	if nb != nil {
		n = nb
	}
	return p.c.KV.Set(key, n, pluginapi.SetAtomic(o))
}

func (p pluginKV) Delete(key string) error { return p.c.KV.Delete(key) }

const pageSize = 100

// ListKeys pages through all keys. pluginapi.WithPrefix filters after paging, so a
// short filtered page does not mean the end; filter here and stop on a short raw page.
func (p pluginKV) ListKeys(prefix string) ([]string, error) {
	var out []string
	for page := 0; ; page++ {
		keys, err := p.c.KV.ListKeys(page, pageSize)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if strings.HasPrefix(k, prefix) {
				out = append(out, k)
			}
		}
		if len(keys) < pageSize {
			return out, nil
		}
	}
}
