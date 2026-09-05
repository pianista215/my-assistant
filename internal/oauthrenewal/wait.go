package oauthrenewal

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

// waitPollInterval is how often WaitForToken re-polls. A fixed interval
// rather than backing off further on "slow_down" (unlike RFC 8628's own
// recommendation) — an accepted simplification since WaitForToken is only
// used by cmd/deviceauthsetup, a one-time interactive tool where a few
// extra polls cost nothing; internal/server's own Poll usage (at most
// once per hourly ESP32 request) never comes close to needing backoff
// either. A package variable, not a WaitForToken parameter, so this
// package's own tests can turn it down without exposing a public knob
// nobody else needs.
var waitPollInterval = 5 * time.Second

// WaitForToken blocks, polling p every waitPollInterval, until the user
// approves or denies it or its device/user code expires. For interactive
// command-line tools (cmd/deviceauthsetup) where blocking is fine — unlike
// internal/server, which only ever calls Poll once per incoming HTTP
// request (see Poll's doc comment).
func (p *PendingAuth) WaitForToken(ctx context.Context) (*oauth2.Token, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitPollInterval):
		}
		if p.Expired() {
			return nil, fmt.Errorf("oauthrenewal: device code expired before it was approved")
		}
		token, done, err := p.Poll(ctx)
		switch {
		case err != nil && done:
			return nil, err
		case done:
			return token, nil
		}
		// Still pending (or a transient poll error) — keep waiting.
	}
}
