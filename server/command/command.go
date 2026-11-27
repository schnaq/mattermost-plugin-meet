// Package command implements the /meet slash command.
package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/schnaq/mattermost-plugin-meet/server/huddle"
)

// Trigger is the slash command's name.
const Trigger = "meet"

// Service is the huddle surface the command uses; *huddle.Service implements it.
type Service interface {
	Start(ctx context.Context, userID, channelID string) (huddle.StartResult, error)
	Invite(ctx context.Context, userID, huddleID string, targetIDs []string) error
	End(ctx context.Context, userID, huddleID string) error
	Active(userID string) ([]huddle.HuddleView, error)
}

// Users is the subset of plugin.API the command needs.
type Users interface {
	GetUser(userID string) (*model.User, *model.AppError)
	GetUserByUsername(username string) (*model.User, *model.AppError)
	GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError)
	LogError(msg string, keyValuePairs ...any)
}

type Handler struct {
	svc       Service
	users     Users
	pluginURL string
}

func New(svc Service, users Users, pluginURL string) *Handler {
	return &Handler{svc: svc, users: users, pluginURL: pluginURL}
}

// Definition is the command to register, with autocomplete hints.
func Definition() *model.Command {
	ad := model.NewAutocompleteData(Trigger, "[@user ...|end]", "Start a huddle in this channel")
	ad.AddCommand(model.NewAutocompleteData("end", "", "End the huddle for everyone"))
	return &model.Command{Trigger: Trigger, AutoComplete: true, AutoCompleteDesc: "Start a huddle in this channel", AutoCompleteHint: "[@user ...|end]", AutocompleteData: ad}
}

// Execute runs "/meet", "/meet @a @b" or "/meet end". Every answer is ephemeral.
func (h *Handler) Execute(ctx context.Context, args *model.CommandArgs) *model.CommandResponse {
	locale := huddle.Locale(h.users.GetUser, args.UserId)
	fields := strings.Fields(args.Command)
	if len(fields) > 0 {
		fields = fields[1:] // "/meet"
	}
	msg, joinID, err := h.run(ctx, args, locale, fields)
	if err != nil {
		errMsg, unexpected := huddle.ErrorText(locale, err)
		if unexpected {
			h.users.LogError("meet command failed", "err", err.Error())
		}
		msg = strings.TrimSpace(msg + "\n" + errMsg)
	}
	resp := &model.CommandResponse{ResponseType: model.CommandResponseTypeEphemeral, Text: msg}
	if joinID != "" {
		resp.Attachments = []*model.MessageAttachment{{Actions: []*model.PostAction{huddle.JoinAction(locale, h.pluginURL, joinID)}}}
	}
	return resp
}

// run returns the text and, when a join button belongs to it, the huddle ID. A failed invite
// still returns both, so the starter can join the huddle that is running anyway.
func (h *Handler) run(ctx context.Context, args *model.CommandArgs, locale string, fields []string) (msg, joinID string, err error) {
	if len(fields) == 1 && fields[0] == "end" {
		return h.end(ctx, args, locale)
	}
	var ids []string
	for _, f := range fields {
		name, ok := strings.CutPrefix(f, "@")
		if !ok {
			return huddle.Text(locale, "cmd_usage"), "", nil
		}
		u, appErr := h.users.GetUserByUsername(name)
		if appErr != nil {
			return fmt.Sprintf(huddle.Text(locale, "cmd_unknown_user"), name), "", nil
		}
		if _, appErr := h.users.GetChannelMember(args.ChannelId, u.Id); appErr != nil {
			return fmt.Sprintf(huddle.Text(locale, "cmd_not_member"), name), "", nil
		}
		ids = append(ids, u.Id)
	}
	res, err := h.svc.Start(ctx, args.UserId, args.ChannelId)
	if err != nil {
		return "", "", err
	}
	// A new DM/GM huddle already rang every member, the targets included.
	if len(ids) > 0 && !res.Rang {
		err = h.svc.Invite(ctx, args.UserId, res.Huddle.ID, ids)
	}
	return huddle.Text(locale, "cmd_started"), res.Huddle.ID, err
}

func (h *Handler) end(ctx context.Context, args *model.CommandArgs, locale string) (string, string, error) {
	views, err := h.svc.Active(args.UserId)
	if err != nil {
		return "", "", err
	}
	for _, v := range views {
		if v.ChannelID == args.ChannelId {
			if err = h.svc.End(ctx, args.UserId, v.ID); err != nil {
				return "", "", err
			}
			return huddle.Text(locale, "cmd_ended"), "", nil
		}
	}
	return huddle.Text(locale, "cmd_none"), "", nil
}
