package huddle

import (
	"errors"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

var texts = map[string]map[string]string{
	"en": {
		"card_message": ":headphones: Huddle started — join with the button below.",
		"card_title":   "Huddle",
		"join":         "Join",
		"huddle_ended": "This huddle has ended.",
		"summary":      ":headphones: Huddle ended after %s.",
		"recording":    ":movie_camera: Recording: %s",
		"transcript":   ":page_facing_up: Transcript: %s",
		"smart_notes":  ":memo: Meeting notes: %s",
		"invite":       "@%s invites you to the huddle in **%s**",
		"invite_dm":    "@%s invites you to a huddle",
		"missed_from":  "Missed huddle call from %s",
		"missed":       "Missed huddle call",

		"err_bad_request":  "Invalid request.",
		"err_not_member":   "Everyone you invite must be a member of this channel.",
		"err_forbidden":    "You are not allowed to do that.",
		"err_ended":        "This huddle has ended.",
		"err_no_owner":     "No Google account is configured for you. Ask an admin to add you.",
		"err_rate_limited": "You invited these people a moment ago. Please wait a bit.",
		"err_not_found":    "Not found.",
		"err_unavailable":  "Google Meet is unavailable or not configured. Please try again later.",
		"err_internal":     "Something went wrong.",

		"cmd_started":      ":headphones: A huddle is running. Join with the button below.",
		"cmd_ended":        "Huddle ended.",
		"cmd_none":         "There is no huddle in this channel.",
		"cmd_unknown_user": "Unknown user @%s.",
		"cmd_not_member":   "@%s is not a member of this channel.",
		"cmd_usage":        "Usage: /meet, /meet @user [@user ...], /meet end",
	},
	"de": {
		"card_message": ":headphones: Huddle gestartet – über den Button beitreten.",
		"card_title":   "Huddle",
		"join":         "Beitreten",
		"huddle_ended": "Dieses Huddle ist beendet.",
		"summary":      ":headphones: Huddle beendet nach %s.",
		"recording":    ":movie_camera: Aufzeichnung: %s",
		"transcript":   ":page_facing_up: Transkript: %s", //nolint:misspell // German
		"smart_notes":  ":memo: Notizen: %s",
		"invite":       "@%s lädt dich ins Huddle in **%s** ein",
		"invite_dm":    "@%s lädt dich zu einem Huddle ein",
		"missed_from":  "Verpasster Huddle-Anruf von %s",
		"missed":       "Verpasster Huddle-Anruf",

		"err_bad_request":  "Ungültige Anfrage.",
		"err_not_member":   "Alle Eingeladenen müssen Mitglied dieses Channels sein.",
		"err_forbidden":    "Das darfst du nicht.",
		"err_ended":        "Dieses Huddle ist beendet.",
		"err_no_owner":     "Für dich ist kein Google-Konto hinterlegt. Bitte einen Admin, dich einzutragen.",
		"err_rate_limited": "Du hast diese Personen gerade erst eingeladen. Bitte kurz warten.",
		"err_not_found":    "Nicht gefunden.",
		"err_unavailable":  "Google Meet ist nicht erreichbar oder nicht konfiguriert. Bitte später erneut versuchen.",
		"err_internal":     "Etwas ist schiefgelaufen.",

		"cmd_started":      ":headphones: Ein Huddle läuft. Über den Button beitreten.",
		"cmd_ended":        "Huddle beendet.",
		"cmd_none":         "In diesem Channel läuft kein Huddle.",
		"cmd_unknown_user": "Unbekannter User @%s.",
		"cmd_not_member":   "@%s ist nicht in diesem Channel.",
		"cmd_usage":        "Aufruf: /meet, /meet @user [@user ...], /meet end",
	},
}

// Text returns the server text for key in the recipient's locale ("de", "de-DE", …), defaulting to en.
func Text(locale, key string) string {
	lang, _, _ := strings.Cut(locale, "-")
	if t, ok := texts[lang][key]; ok {
		return t
	}
	return texts["en"][key]
}

// Locale returns the user's locale via getUser (e.g. plugin.API.GetUser), "" (en) if that fails.
func Locale(getUser func(userID string) (*model.User, *model.AppError), userID string) string {
	if u, appErr := getUser(userID); appErr == nil {
		return u.Locale
	}
	return ""
}

var errKeys = []struct {
	err error
	key string
}{
	{ErrForbidden, "err_forbidden"},
	{ErrEnded, "err_ended"},
	{ErrNoOwner, "err_no_owner"},
	{ErrRateLimited, "err_rate_limited"},
	{ErrNotFound, "err_not_found"},
	{ErrNotMember, "err_not_member"}, // before ErrBadRequest, which it wraps
	{ErrBadRequest, "err_bad_request"},
	{ErrMeetUnavailable, "err_unavailable"},
}

// ErrorText returns a user-readable text in locale for a service error. unexpected is true for
// errors that are none of the package's sentinels: they get a generic text, details belong in the logs.
func ErrorText(locale string, err error) (text string, unexpected bool) {
	for _, m := range errKeys {
		if errors.Is(err, m.err) {
			return Text(locale, m.key), false
		}
	}
	return Text(locale, "err_internal"), true
}
