package huddle

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

type wsEvent struct {
	Event     string
	Payload   map[string]any
	Broadcast *model.WebsocketBroadcast
}

// fakeMM is an in-memory MM for tests. Seed with addChannel/addUser, then
// inspect the recorded fields (lock-free reads are fine once the code under test returned).
type fakeMM struct {
	mu sync.Mutex

	channels      map[string]*model.Channel
	members       map[string]map[string]bool // channelID → userID set
	users         map[string]*model.User
	statuses      map[string]*model.Status
	posts         map[string]*model.Post
	created       []*model.Post
	updated       []*model.Post
	ws            []wsEvent
	customStatus  map[string]*model.CustomStatus
	removedStatus []string
	logs          []string
	nextID        int
	failCreate    func(*model.Post) *model.AppError // optional: CreatePost returns its non-nil error
}

func newFakeMM() *fakeMM {
	return &fakeMM{
		channels:     map[string]*model.Channel{},
		members:      map[string]map[string]bool{},
		users:        map[string]*model.User{},
		statuses:     map[string]*model.Status{},
		posts:        map[string]*model.Post{},
		customStatus: map[string]*model.CustomStatus{},
	}
}

func notFound(what string) *model.AppError {
	return model.NewAppError("fake", what+" not found", nil, "", http.StatusNotFound)
}

func (f *fakeMM) addChannel(id string, typ model.ChannelType, userIDs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.channels[id] = &model.Channel{Id: id, Type: typ}
	f.members[id] = map[string]bool{}
	for _, u := range userIDs {
		f.members[id][u] = true
	}
}

func (f *fakeMM) removeMember(channelID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.members[channelID], userID)
}

func (f *fakeMM) addUser(id, first, locale string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[id] = &model.User{Id: id, Username: id, FirstName: first, Locale: locale}
}

func (f *fakeMM) GetChannel(channelID string) (*model.Channel, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.channels[channelID]; ok {
		return c, nil
	}
	return nil, notFound("channel")
}

func (f *fakeMM) GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.members[channelID][userID] {
		return &model.ChannelMember{ChannelId: channelID, UserId: userID}, nil
	}
	return nil, notFound("member")
}

func (f *fakeMM) GetChannelMembers(channelID string, page, perPage int) (model.ChannelMembers, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if page > 0 {
		return nil, nil
	}
	var out model.ChannelMembers
	for u := range f.members[channelID] {
		out = append(out, model.ChannelMember{ChannelId: channelID, UserId: u})
	}
	return out, nil
}

func (f *fakeMM) GetUser(userID string) (*model.User, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[userID]; ok {
		return u, nil
	}
	return nil, notFound("user")
}

func (f *fakeMM) GetUserStatus(userID string) (*model.Status, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.statuses[userID]; ok {
		return s, nil
	}
	return &model.Status{UserId: userID, Status: model.StatusOnline}, nil
}

func (f *fakeMM) GetDirectChannel(userID1, userID2 string) (*model.Channel, *model.AppError) {
	return &model.Channel{Id: model.GetDMNameFromIds(userID1, userID2), Type: model.ChannelTypeDirect}, nil
}

func (f *fakeMM) CreatePost(post *model.Post) (*model.Post, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCreate != nil {
		if appErr := f.failCreate(post); appErr != nil {
			return nil, appErr
		}
	}
	f.nextID++
	p := post.Clone()
	p.Id = fmt.Sprintf("post%d", f.nextID)
	f.posts[p.Id] = p
	f.created = append(f.created, p)
	return p, nil
}

func (f *fakeMM) UpdatePost(post *model.Post) (*model.Post, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.posts[post.Id]; !ok {
		return nil, notFound("post")
	}
	p := post.Clone()
	f.posts[p.Id] = p
	f.updated = append(f.updated, p)
	return p, nil
}

func (f *fakeMM) deletePost(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.posts, id)
}

func (f *fakeMM) PublishWebSocketEvent(event string, payload map[string]any, broadcast *model.WebsocketBroadcast) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ws = append(f.ws, wsEvent{event, payload, broadcast})
}

func (f *fakeMM) UpdateUserCustomStatus(userID string, cs *model.CustomStatus) *model.AppError {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.customStatus[userID] = cs
	return nil
}

func (f *fakeMM) RemoveUserCustomStatus(userID string) *model.AppError {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.customStatus, userID)
	f.removedStatus = append(f.removedStatus, userID)
	return nil
}

func (f *fakeMM) LogError(msg string, keyValuePairs ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, fmt.Sprint(append([]any{msg}, keyValuePairs...)...))
}

var _ MM = (*fakeMM)(nil)

// fakeRinger records ring calls as "channel <huddleID> <postID> <from>" / "users <huddleID> <from> <to...>".
type fakeRinger struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]error // ringUsers returns fail[to[0]]
}

func (r *fakeRinger) ringChannel(_ context.Context, h *Huddle, from string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, fmt.Sprint("channel ", h.ID, " ", h.PostID, " ", from))
	return nil
}

func (r *fakeRinger) ringUsers(_ context.Context, h *Huddle, from string, to []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, fmt.Sprint("users ", h.ID, " ", from, " ", to))
	return r.fail[to[0]]
}

func (r *fakeRinger) expire(_ context.Context, now time.Time, rings []*Ring) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, fmt.Sprint("expire ", now.Format(time.RFC3339), " ", len(rings)))
}

func (r *fakeRinger) settle(string, *Huddle) {}

// fakePresence records "joined <user> <huddleID>" / "left <user> <huddleID>".
type fakePresence struct {
	mu    sync.Mutex
	calls []string
}

func (p *fakePresence) joined(userID string, h *Huddle) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "joined "+userID+" "+h.ID)
}

func (p *fakePresence) left(userID string, h *Huddle) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "left "+userID+" "+h.ID)
}
