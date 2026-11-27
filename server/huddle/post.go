package huddle

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

const cardPostType = "custom_meet_huddle"

// buildCardPost renders the huddle card, posted as the starter. The webapp replaces it with a
// live card; message and attachment are the mobile fallback, in the starter's locale since one
// post serves every channel member. Never contains the Meet link.
func buildCardPost(h *Huddle, pluginURL string) *model.Post {
	locale := h.StarterLocale
	var names []string
	for _, p := range view(h).Participants {
		names = append(names, p.DisplayName)
	}
	p := &model.Post{
		Id:        h.PostID,
		UserId:    h.StarterUserID,
		ChannelId: h.ChannelID,
		Type:      cardPostType,
		Message:   Text(locale, "card_message"),
	}
	p.AddProp("huddle_id", h.ID)
	p.AddProp(model.PostPropsAttachments, []*model.MessageAttachment{{
		Fallback: Text(locale, "card_message"),
		Title:    Text(locale, "card_title"),
		Text:     strings.Join(names, ", "),
		Actions:  []*model.PostAction{JoinAction(locale, pluginURL, h.ID)},
	}})
	return p
}

// buildSummaryPost replaces the card once the huddle ended: duration and everyone ever
// confirmed (names), without the join button. The "ended" prop tells the webapp.
func buildSummaryPost(h *Huddle, names []string, d time.Duration) *model.Post {
	locale := h.StarterLocale
	msg := fmt.Sprintf(Text(locale, "summary"), formatDuration(d))
	p := &model.Post{Id: h.PostID, UserId: h.StarterUserID, ChannelId: h.ChannelID, Type: cardPostType, Message: msg}
	p.AddProp("huddle_id", h.ID)
	p.AddProp("ended", true)
	p.AddProp(model.PostPropsAttachments, []*model.MessageAttachment{{
		Fallback: msg,
		Title:    Text(locale, "card_title"),
		Text:     strings.Join(names, ", "),
	}})
	return p
}

// formatDuration renders d as "5 min" or "1 h 5 min" (same in en and de).
func formatDuration(d time.Duration) string {
	m := max(int(d.Round(time.Minute)/time.Minute), 1)
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

// JoinAction is the Join button; the integration URL is relative, the server routes /plugins/... internally.
func JoinAction(locale, pluginURL, huddleID string) *model.PostAction {
	return &model.PostAction{
		Id:    "join",
		Type:  model.PostActionTypeButton,
		Name:  Text(locale, "join"),
		Style: "primary",
		Integration: &model.PostActionIntegration{
			URL:     pluginURL + "/api/v1/actions/join",
			Context: map[string]any{"huddle_id": huddleID},
		},
	}
}
