package command

import (
	"context"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"

	"github.com/schnaq/mattermost-plugin-meet/server/huddle"
)

type fakeSvc struct {
	started, invited, ended []string
	active                  []huddle.HuddleView
	rang                    bool
	inviteErr               error
}

func (f *fakeSvc) Start(_ context.Context, u, ch string) (huddle.StartResult, error) {
	f.started = append(f.started, u+":"+ch)
	return huddle.StartResult{Huddle: huddle.HuddleView{ID: "h1", ChannelID: ch}, Rang: f.rang}, nil
}

func (f *fakeSvc) Invite(_ context.Context, u, id string, ids []string) error {
	f.invited = append(f.invited, u+":"+id)
	f.invited = append(f.invited, ids...)
	return f.inviteErr
}

func (f *fakeSvc) End(_ context.Context, u, id string) error {
	f.ended = append(f.ended, u+":"+id)
	return nil
}
func (f *fakeSvc) Active(string) ([]huddle.HuddleView, error) { return f.active, nil }

type fakeUsers struct{}

func (fakeUsers) GetUser(id string) (*model.User, *model.AppError) {
	return &model.User{Id: id, Locale: "en"}, nil
}

func (fakeUsers) LogError(string, ...any) {}
func (fakeUsers) GetChannelMember(_, userID string) (*model.ChannelMember, *model.AppError) {
	if userID == "id-outsider" {
		return nil, model.NewAppError("x", "nf", nil, "", 404)
	}
	return &model.ChannelMember{}, nil
}

func (fakeUsers) GetUserByUsername(name string) (*model.User, *model.AppError) {
	if name == "ghost" {
		return nil, model.NewAppError("x", "nf", nil, "", 404)
	}
	return &model.User{Id: "id-" + name}, nil
}

func run(svc *fakeSvc, cmd string) *model.CommandResponse {
	h := New(svc, fakeUsers{}, "/plugins/com.schnaq.meet")
	return h.Execute(context.Background(), &model.CommandArgs{Command: cmd, UserId: "u1", ChannelId: "c1"})
}

func TestCommandStartHasJoinButtonNoURL(t *testing.T) {
	svc := &fakeSvc{}
	r := run(svc, "/meet")
	require.Equal(t, model.CommandResponseTypeEphemeral, r.ResponseType)
	require.Equal(t, []string{"u1:c1"}, svc.started)
	require.Empty(t, svc.invited)
	a := r.Attachments[0].Actions[0]
	require.Equal(t, "/plugins/com.schnaq.meet/api/v1/actions/join", a.Integration.URL)
	require.Equal(t, "h1", a.Integration.Context["huddle_id"])
}

func TestCommandParsesInviteMentions(t *testing.T) {
	svc := &fakeSvc{}
	r := run(svc, "/meet @anna  @ben")
	require.Equal(t, model.CommandResponseTypeEphemeral, r.ResponseType)
	require.Equal(t, []string{"u1:c1"}, svc.started)
	require.Equal(t, []string{"u1:h1", "id-anna", "id-ben"}, svc.invited)

	svc = &fakeSvc{}
	r = run(svc, "/meet @anna @ghost")
	require.Contains(t, r.Text, "@ghost")
	require.Empty(t, svc.started)
}

func TestCommandEnd(t *testing.T) {
	svc := &fakeSvc{active: []huddle.HuddleView{{ID: "other", ChannelID: "c2"}, {ID: "h7", ChannelID: "c1"}}}
	r := run(svc, "/meet end")
	require.Equal(t, model.CommandResponseTypeEphemeral, r.ResponseType)
	require.Equal(t, []string{"u1:h7"}, svc.ended)

	svc = &fakeSvc{}
	require.Contains(t, run(svc, "/meet end").Text, huddle.Text("en", "cmd_none"))
	require.Empty(t, svc.ended)
}

func TestCommandUsage(t *testing.T) {
	svc := &fakeSvc{}
	require.Equal(t, huddle.Text("en", "cmd_usage"), run(svc, "/meet bogus").Text)
	require.Empty(t, svc.started)
}

func TestCommandAutocompleteValid(t *testing.T) {
	require.NoError(t, Definition().AutocompleteData.IsValid())
}

func TestCommandInviteNonMemberStartsNothing(t *testing.T) {
	svc := &fakeSvc{}
	r := run(svc, "/meet @anna @outsider")
	require.Contains(t, r.Text, "@outsider is not a member")
	require.Empty(t, svc.started)
	require.Empty(t, svc.invited)
}

func TestCommandInviteFailureKeepsJoinButton(t *testing.T) {
	svc := &fakeSvc{inviteErr: huddle.ErrRateLimited}
	r := run(svc, "/meet @anna")
	require.Contains(t, r.Text, huddle.Text("en", "err_rate_limited"))
	require.Len(t, r.Attachments, 1)
	require.Equal(t, "h1", r.Attachments[0].Actions[0].Integration.Context["huddle_id"])
}

func TestCommandNewDMHuddleSkipsInvite(t *testing.T) {
	svc := &fakeSvc{rang: true}
	r := run(svc, "/meet @anna")
	require.Equal(t, []string{"u1:c1"}, svc.started)
	require.Empty(t, svc.invited)
	require.Len(t, r.Attachments, 1)
}
