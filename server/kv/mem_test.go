package kv

import (
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

func TestMemCompareAndSet(t *testing.T) {
	m := NewMem()
	ok, err := m.CompareAndSet("k", nil, "a")
	if err != nil || !ok {
		t.Fatalf("create-only: ok=%v err=%v", ok, err)
	}
	if ok, _ := m.CompareAndSet("k", nil, "b"); ok {
		t.Fatal("create-only must fail when present")
	}
	if ok, _ := m.CompareAndSet("k", "x", "b"); ok {
		t.Fatal("conflict must fail")
	}
	if ok, _ := m.CompareAndSet("k", "a", "b"); !ok {
		t.Fatal("matching old must succeed")
	}
	var s string
	if found, _ := m.Get("k", &s); !found || s != "b" {
		t.Fatalf("got %q found=%v", s, found)
	}
	if ok, _ := m.CompareAndSet("k", "b", nil); !ok {
		t.Fatal("delete via nil must succeed")
	}
	if found, _ := m.Get("k", &s); found {
		t.Fatal("key should be gone")
	}
	_ = m.Set("p:1", 1)
	_ = m.Set("p:2", 2)
	_ = m.Set("q:1", 3)
	if keys, _ := m.ListKeys("p:"); len(keys) != 2 {
		t.Fatalf("keys=%v", keys)
	}
}

func TestMemTTL(t *testing.T) {
	now := time.Unix(0, 0)
	m := NewMemClock(func() time.Time { return now })
	_ = m.SetWithTTL("t:1", 1, time.Minute)
	_ = m.Set("t:2", 2)
	now = now.Add(59 * time.Second)
	if found, _ := m.Get("t:1", new(int)); !found {
		t.Fatal("key must live until its TTL")
	}
	now = now.Add(time.Second)
	if found, _ := m.Get("t:1", new(int)); found {
		t.Fatal("expired key must read as absent")
	}
	if keys, _ := m.ListKeys("t:"); len(keys) != 1 || keys[0] != "t:2" {
		t.Fatalf("keys=%v", keys)
	}
	if ok, _ := m.CompareAndSet("t:1", nil, 3); !ok {
		t.Fatal("expired key must count as absent for CompareAndSet")
	}
	_ = m.SetWithTTL("t:2", 2, time.Second)
	_ = m.Set("t:2", 2) // a plain Set clears the expiry
	now = now.Add(time.Hour)
	if found, _ := m.Get("t:2", new(int)); !found {
		t.Fatal("plain Set must not expire")
	}
}

func TestPluginKVSetWithTTL(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVSetWithOptions", "k", []byte(`"v"`), model.PluginKVSetOptions{ExpireInSeconds: 3600}).Return(true, nil).Once()
	api.On("KVSetWithOptions", "k", []byte(`"v"`), model.PluginKVSetOptions{}).Return(true, nil).Once()
	k := New(pluginapi.NewClient(api, nil))
	if err := k.SetWithTTL("k", "v", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := k.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	api.AssertExpectations(t)
}
