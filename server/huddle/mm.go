package huddle

import "github.com/mattermost/mattermost/server/public/model"

// MM is the exact subset of plugin.API the huddle package uses.
type MM interface {
	GetChannel(channelID string) (*model.Channel, *model.AppError)
	GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError)
	GetChannelMembers(channelID string, page, perPage int) (model.ChannelMembers, *model.AppError)
	GetUser(userID string) (*model.User, *model.AppError)
	GetUserStatus(userID string) (*model.Status, *model.AppError)
	GetDirectChannel(userID1, userID2 string) (*model.Channel, *model.AppError)
	CreatePost(post *model.Post) (*model.Post, *model.AppError)
	UpdatePost(post *model.Post) (*model.Post, *model.AppError)
	PublishWebSocketEvent(event string, payload map[string]any, broadcast *model.WebsocketBroadcast)
	UpdateUserCustomStatus(userID string, customStatus *model.CustomStatus) *model.AppError
	RemoveUserCustomStatus(userID string) *model.AppError
	LogError(msg string, keyValuePairs ...any)
}
