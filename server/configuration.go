package main

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/schnaq/mattermost-plugin-meet/server/google"
	"github.com/schnaq/mattermost-plugin-meet/server/huddle"
)

// configuration captures the plugin's external configuration as exposed in the Mattermost server
// configuration, as well as values computed from the configuration. Any public fields will be
// deserialized from the Mattermost server configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// configuration can change at any time, access to the configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
//
// If you add non-reference types to your configuration struct, be sure to rewrite Clone as a deep
// copy appropriate for your types.
type configuration struct {
	ServiceAccountJSON string
	GoogleAccounts     string
	FallbackOwnerEmail string
	StatusText         string
	StatusEmoji        string
}

// Clone shallow copies the configuration. Your implementation may require a deep copy if
// your configuration has reference types.
func (c *configuration) Clone() *configuration {
	clone := *c
	return &clone
}

// OnConfigurationChange is invoked when configuration changes may have been made.
func (p *Plugin) OnConfigurationChange() error {
	configuration := new(configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	accounts, errs := parseGoogleAccounts(configuration.GoogleAccounts, func(username string) (string, error) {
		u, appErr := p.API.GetUserByUsername(username)
		if appErr != nil {
			return "", appErr
		}
		return u.Id, nil
	})
	for _, err := range errs {
		p.API.LogWarn("GoogleAccounts: " + err.Error())
	}

	// Both are set under one lock so huddleConfig always sees a consistent pair.
	p.configurationLock.Lock()
	p.configuration = configuration
	p.googleAccounts = accounts
	p.configurationLock.Unlock()

	p.rebuildMeet(configuration.ServiceAccountJSON)
	// Google user mappings may be stale under the new accounts/key. Runs before OnActivate too.
	if err := p.store().ClearGoogleCache(); err != nil {
		p.API.LogError("clearing the Google user cache failed", "err", err.Error())
	}
	return nil
}

// newMeet builds the Google client; the demo build (-tags demo) swaps in a fake.
var newMeet = func(key []byte) (google.Meet, error) { return google.New(key) }

// rebuildMeet swaps the Google client. A missing or invalid key leaves the plugin active
// but unconfigured: starting a huddle then fails with a clear error.
func (p *Plugin) rebuildMeet(serviceAccountJSON string) {
	if strings.TrimSpace(serviceAccountJSON) == "" {
		p.meet.Set(nil)
		return
	}
	m, err := newMeet([]byte(serviceAccountJSON))
	if err != nil {
		p.API.LogError("ServiceAccountJSON is invalid; huddles are disabled", "err", err.Error())
		p.meet.Set(nil)
		return
	}
	p.meet.Set(m)
}

// huddleConfig snapshots the configuration, google accounts and bot for the huddle service.
// The plugin URL is relative: the server routes /plugins/... integration actions internally.
func (p *Plugin) huddleConfig() huddle.Config {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()
	c := p.configuration
	if c == nil {
		c = &configuration{}
	}
	return huddle.Config{
		GoogleAccounts: p.googleAccounts, FallbackOwnerEmail: c.FallbackOwnerEmail,
		StatusText: c.StatusText, StatusEmoji: c.StatusEmoji,
		BotUserID: p.botUserID, PluginURL: pluginURL,
	}
}

// parseGoogleAccounts parses "username = google-email" lines into userID -> google-email.
// Blank lines and #-comments are skipped; invalid lines yield an error each, the rest still apply.
// The Mattermost user email is deliberately never used.
func parseGoogleAccounts(raw string, lookup func(username string) (string, error)) (map[string]string, []error) {
	m := map[string]string{}
	var errs []error
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		user, email, ok := strings.Cut(line, "=")
		user, email = strings.TrimSpace(user), strings.TrimSpace(email)
		if !ok || user == "" || !strings.Contains(email, "@") {
			errs = append(errs, errors.Errorf("invalid line %q: want username=google-email", line))
			continue
		}
		id, err := lookup(user)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "unknown user %q", user))
			continue
		}
		m[id] = email
	}
	return m, errs
}
