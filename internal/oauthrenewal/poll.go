package oauthrenewal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// deviceTokenResponse is the subset of RFC 8628's token-endpoint response
// this package cares about.
type deviceTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
}

// Poll makes a single, non-blocking attempt to exchange p's device code
// for a token — decoded by hand rather than through
// oauth2.Config.DeviceAccessToken, which blocks in its own polling loop
// internally and isn't usable from within a single HTTP request. There's
// no need to respect RFC 8628's polling interval here either: internal/
// server calls Poll at most once per incoming /api/v1/display request
// (i.e. about once an hour, driven by the ESP32's poll cadence), already
// far slower than Google's minimum interval.
//
// The three-way result:
//   - (token, true, nil): the user approved it — persist token and stop
//     polling this device code.
//   - (nil, false, nil): still pending ("authorization_pending" /
//     "slow_down") — keep showing the same code and try again later.
//   - (nil, true, err): permanently failed (denied, or the code expired
//     server-side) — give up on this device code; a fresh one is needed.
//   - (nil, false, err): the poll request itself failed (network/decode
//     error) — transient, keep the same device code and just try again
//     later, same as the still-pending case.
func (p *PendingAuth) Poll(ctx context.Context) (token *oauth2.Token, done bool, err error) {
	v := url.Values{
		"client_id":     {p.config.ClientID},
		"client_secret": {p.config.ClientSecret},
		"device_code":   {p.DeviceCode},
		"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.Endpoint.TokenURL, strings.NewReader(v.Encode()))
	if err != nil {
		return nil, false, fmt.Errorf("oauthrenewal: building poll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("oauthrenewal: polling token endpoint: %w", err)
	}
	defer res.Body.Close()

	var body deviceTokenResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, false, fmt.Errorf("oauthrenewal: decoding poll response: %w", err)
	}

	switch body.Error {
	case "":
		if body.AccessToken == "" {
			return nil, false, fmt.Errorf("oauthrenewal: poll response had neither an error nor an access_token")
		}
		if body.RefreshToken == "" {
			// Can't persist a renewed credential without one — treat the
			// same as a hard failure rather than silently keeping the
			// (already-dead) old refresh token.
			return nil, true, fmt.Errorf("oauthrenewal: poll response had no refresh_token")
		}
		return &oauth2.Token{
			AccessToken:  body.AccessToken,
			RefreshToken: body.RefreshToken,
			TokenType:    body.TokenType,
			Expiry:       time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
		}, true, nil
	case "authorization_pending", "slow_down":
		return nil, false, nil
	default:
		return nil, true, fmt.Errorf("oauthrenewal: device authorization failed: %s", body.Error)
	}
}
