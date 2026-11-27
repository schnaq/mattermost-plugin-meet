package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"

	"github.com/schnaq/mattermost-plugin-meet/server/api"
	"github.com/schnaq/mattermost-plugin-meet/server/command"
	"github.com/schnaq/mattermost-plugin-meet/server/google"
	"github.com/schnaq/mattermost-plugin-meet/server/huddle"
	"github.com/schnaq/mattermost-plugin-meet/server/kv"
)

const (
	// pluginURL is relative on purpose: the server routes /plugins/... integration actions internally.
	pluginURL    = "/plugins/com.schnaq.meet"
	tickInterval = 2 * time.Second
	// commandTimeout bounds a /meet run (Google calls included).
	commandTimeout = 30 * time.Second
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// httpHandler serves the routes in server/api.
	httpHandler *api.Handler
	command     *command.Handler
	svc         *huddle.Service
	meet        google.Holder // swapped on configuration change

	// configurationLock guards configuration, googleAccounts and botUserID.
	configurationLock sync.RWMutex
	configuration     *configuration
	googleAccounts    map[string]string // Mattermost user ID to google email
	botUserID         string

	stopTick context.CancelFunc
	tickDone chan struct{}
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	client := pluginapi.NewClient(p.API, p.Driver)
	botID, err := client.Bot.EnsureBot(&model.Bot{Username: "huddle", DisplayName: "Huddle", Description: "Huddle invitations"})
	if err != nil {
		return errors.Wrap(err, "failed to ensure bot")
	}
	p.configurationLock.Lock()
	p.botUserID = botID
	p.configurationLock.Unlock()

	p.svc = huddle.NewService(p.API, &p.meet, p.store(), p.huddleConfig, time.Now)
	p.httpHandler = api.NewHandler(p.API, p.svc)
	p.command = command.New(p.svc, p.API, pluginURL)
	if err = p.API.RegisterCommand(command.Definition()); err != nil {
		return errors.Wrap(err, "failed to register command")
	}

	mu, err := cluster.NewMutex(p.API, "huddle-tick")
	if err != nil {
		return errors.Wrap(err, "failed to create tick mutex")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.stopTick, p.tickDone = cancel, make(chan struct{})
	go p.runTicker(ctx, mu, p.tickDone)
	return nil
}

// store builds the KV-backed huddle store; also used by OnConfigurationChange, which runs before OnActivate.
func (p *Plugin) store() huddle.Store {
	return huddle.NewStore(kv.New(pluginapi.NewClient(p.API, p.Driver)))
}

// OnDeactivate stops the ticker and waits for a running tick to finish.
func (p *Plugin) OnDeactivate() error {
	if p.stopTick != nil {
		p.stopTick()
		<-p.tickDone
	}
	return nil
}

// runTicker ticks sequentially (a slow tick delays the next one, never overlaps it).
func (p *Plugin) runTicker(ctx context.Context, mu *cluster.Mutex, done chan<- struct{}) {
	defer close(done)
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.tick(ctx, mu)
		}
	}
}

// tick runs on one node only: the others fail to get the cluster lock quickly and skip.
func (p *Plugin) tick(ctx context.Context, mu *cluster.Mutex) {
	defer func() {
		if r := recover(); r != nil {
			p.API.LogError("huddle tick panicked", "panic", r)
		}
	}()
	if ctx.Err() != nil {
		return
	}
	lockCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if mu.LockWithContext(lockCtx) != nil {
		return
	}
	defer mu.Unlock()
	p.svc.Tick(ctx)
}

// ServeHTTP is invoked for requests to /plugins/{plugin_id}.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.httpHandler.ServeHTTP(c, w, r)
}

// ExecuteCommand runs /meet.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	return p.command.Execute(ctx, args), nil
}
