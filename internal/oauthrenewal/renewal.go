// Package oauthrenewal renews the shared Google OAuth credential
// (internal/calendar's, internal/shoppinglist's and internal/weeklymenu's
// GOOGLE_CREDENTIALS_FILE) without a browser on the machine running the
// server — using the OAuth 2.0 Device Authorization Grant (RFC 8628)
// instead of the loopback-redirect flow cmd/oauthsetup uses for the
// initial one-time login.
//
// This exists because the OAuth consent screen for this project can't be
// published to "In production" without a privacy policy / verified
// domain — infeasible for a personal, no-domain project — so Google caps
// every issued refresh token at 7 days while the app stays in "Testing".
// That expiry is unavoidable either way; this package's job is only to
// make renewing it not require going back to a PC: internal/server shows
// a QR code (built from this package's PendingAuth) on the e-ink panel
// itself when the stored credential has expired, so renewing is "scan
// with your phone" instead of "run cmd/oauthsetup at your desk".
//
// The device flow requires its own OAuth client, of Google Cloud
// Console's "TVs and Limited Input devices" type (a Desktop-type client,
// like cmd/oauthsetup's, gets invalid_client on this endpoint) — set up
// once via cmd/deviceauthsetup, see that command's doc comment. After
// that one-time bootstrap, this package reads that client's
// client_id/client_secret back out of the very credentials file it's
// renewing (see ClientCredentials) rather than needing separate
// configuration.
package oauthrenewal

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/drive/v3"
)

// Scopes mirrors what internal/calendar, internal/shoppinglist and
// internal/weeklymenu each request — the renewed credential is shared
// between all three, so it must cover the union of what they need.
var Scopes = []string{calendar.CalendarReadonlyScope, drive.DriveFileScope}

// PendingAuth is one in-flight device authorization request: the
// device_code/user_code pair Google issued, and everything needed to poll
// for its completion.
type PendingAuth struct {
	config *oauth2.Config

	// ClientID and ClientSecret are the device-flow OAuth client's own —
	// exposed so the caller can pass them straight to SaveCredentials
	// once Poll succeeds, without reaching back into config.
	ClientID     string
	ClientSecret string

	DeviceCode string
	UserCode   string
	// VerificationURL is what the QR code/link should point at: Google's
	// VerificationURIComplete when present, since then scanning and
	// confirming is all the user has to do (the code is already embedded
	// in the URL); otherwise the bare VerificationURI, in which case
	// NeedsUserCode is true and the caller must also display UserCode for
	// the user to type in by hand.
	VerificationURL string
	NeedsUserCode   bool

	expiry time.Time
}

// Expired reports whether this device/user code pair is past Google's
// expiry window (typically ~30 minutes) — a fresh one must be requested
// via StartDeviceAuth instead of continuing to Poll this one.
func (p *PendingAuth) Expired() bool {
	return time.Now().After(p.expiry)
}

// StartDeviceAuth begins a new device authorization request: reads the
// current client_id/client_secret out of credentialsFile (see
// ClientCredentials) and asks Google for a device_code/user_code pair.
func StartDeviceAuth(ctx context.Context, credentialsFile string) (*PendingAuth, error) {
	clientID, clientSecret, err := ClientCredentials(credentialsFile)
	if err != nil {
		return nil, err
	}
	return StartDeviceAuthWithEndpoint(ctx, clientID, clientSecret, google.Endpoint)
}

// StartDeviceAuthWithEndpoint is StartDeviceAuth's endpoint-injectable
// core: exported so both this package's own tests and internal/server's
// (which needs a real, working *PendingAuth pointed at a fake Google to
// exercise its polling/renewal orchestration without hitting the network)
// can point it at an httptest.Server instead of Google's real endpoints.
func StartDeviceAuthWithEndpoint(ctx context.Context, clientID, clientSecret string, endpoint oauth2.Endpoint) (*PendingAuth, error) {
	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     endpoint,
		Scopes:       Scopes,
	}
	resp, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauthrenewal: requesting device code: %w", err)
	}

	verificationURL := resp.VerificationURIComplete
	needsUserCode := verificationURL == ""
	if needsUserCode {
		verificationURL = resp.VerificationURI
	}

	return &PendingAuth{
		config:          cfg,
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		DeviceCode:      resp.DeviceCode,
		UserCode:        resp.UserCode,
		VerificationURL: verificationURL,
		NeedsUserCode:   needsUserCode,
		expiry:          resp.Expiry,
	}, nil
}
