//go:build !wasm

package sse

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
)

// ErrNoSessionUser refuses a notices connection that carries no signed-in user: a notice is always for one person.
var ErrNoSessionUser = fmt.Err("sse: the notices stream needs a signed-in user")

// userChannelPrefix names the channel that holds every connection of one user.
const userChannelPrefix = "user:"

// UserNotices is the stream that carries notices to signed-in users: each connection joins only the channel of its
// session's user, and Notify reaches every browser where that user is signed in — nobody else's. A notice carries no
// data: the screen that listens (Subscriber, in the browser) reads what changed through its normal caller, which
// checks permissions as any other read.
//
//	notices := sse.NewUserNotices("/notices")
//	notices.MountAPI(r)
//	notices.Notify(userID, "clinical_encounter.agenda.changed")
type UserNotices struct {
	server *SSEServer
}

// NewUserNotices builds the notices stream served at path, open only to signed-in users.
func NewUserNotices(path string) *UserNotices {
	n := &UserNotices{}
	n.server = New(&Config{}).Server(&ServerConfig{
		Path:                path,
		Access:              model.AccessAuthenticated,
		ChannelProvider:     n,
		ClientChannelBuffer: 16,
	})
	return n
}

// ModelName is the module identity (router.APIModule).
func (n *UserNotices) ModelName() string { return ModuleName }

// MountAPI registers the stream route (router.APIModule).
func (n *UserNotices) MountAPI(r router.Router) { n.server.MountAPI(r) }

// Notify sends the notice topic to every browser where userID is signed in. An empty user or topic sends nothing.
func (n *UserNotices) Notify(userID, topic string) {
	if userID == "" || topic == "" {
		return
	}
	n.server.PublishEvent(topic, nil, userChannelPrefix+userID)
}

// ResolveChannels joins a connection to its session user's channel only (ChannelProvider).
func (n *UserNotices) ResolveChannels(ctx router.Context) ([]string, error) {
	id := ctx.UserID()
	if id == "" {
		return nil, ErrNoSessionUser
	}
	return []string{userChannelPrefix + id}, nil
}

var (
	_ router.APIModule = (*UserNotices)(nil)
	_ ChannelProvider  = (*UserNotices)(nil)
)
