package oauthrenewal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"cloud.google.com/go/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

// ClientOption builds the google.golang.org/api client option that
// internal/calendar, internal/shoppinglist and internal/weeklymenu
// authenticate with, from the authorized_user credentials file at path —
// used instead of option.WithCredentialsFile because of how that one
// caches access tokens.
//
// option.WithCredentialsFile goes through cloud.google.com/go/auth's
// default token cache, which refreshes *asynchronously*: once an access
// token is within 3m45s of expiring it's "stale", and the cache keeps
// handing out that same token while it refreshes in the background. The
// ESP32 polls exactly hourly and an access token lives ~3599s, so every
// poll lands right at the previous token's expiry: often it arrives a
// second or two before it, gets the stale token, and the API rejects it
// mid-request with a 401 "Invalid Credentials" — showing as a spurious
// error in whichever section's call crossed the boundary (the weekly
// menu, the shopping list, or the calendar, which triggers the QR
// reauthorization screen), while the refresh token itself is perfectly
// fine and the next request works.
//
// The cache built here refreshes *synchronously* instead
// (DisableAsyncRefresh): a request finding a token with less than 3m45s
// left blocks on a real refresh first, so a token about to expire is
// never sent. The httptransport layer only wraps a TokenProvider in its
// own (asynchronous) cache when it isn't one already, so this one is used
// as-is.
func ClientOption(path string) (option.ClientOption, error) {
	creds, err := loadStoredCredentials(path)
	if err != nil {
		return nil, err
	}
	if creds.RefreshToken == "" {
		return nil, fmt.Errorf("oauthrenewal: %s has no refresh_token", path)
	}
	tp := newBlockingTokenProvider(creds.ClientID, creds.ClientSecret, creds.RefreshToken, google.Endpoint)
	return option.WithAuthCredentials(auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: tp})), nil
}

// newBlockingTokenProvider is ClientOption's endpoint-injectable core
// (for tests): refreshToken exchanged at endpoint, cached with
// synchronous refresh.
func newBlockingTokenProvider(clientID, clientSecret, refreshToken string, endpoint oauth2.Endpoint) auth.TokenProvider {
	raw := &refreshTokenProvider{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     endpoint,
			Scopes:       Scopes,
		},
		refreshToken: refreshToken,
	}
	return auth.NewCachedTokenProvider(raw, &auth.CachedTokenProviderOptions{DisableAsyncRefresh: true})
}

// refreshTokenProvider performs one refresh-token exchange per call, with
// no caching of its own (the cache wrapping it decides when to call). A
// dead refresh token surfaces as *oauth2.RetrieveError{ErrorCode:
// "invalid_grant"}, one of the shapes IsReauthError recognizes.
type refreshTokenProvider struct {
	config       *oauth2.Config
	refreshToken string
}

func (p *refreshTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	// A TokenSource seeded with only a refresh token (no access token)
	// always performs a refresh on its first Token call.
	tok, err := p.config.TokenSource(ctx, &oauth2.Token{RefreshToken: p.refreshToken}).Token()
	if err != nil {
		return nil, fmt.Errorf("oauthrenewal: refreshing access token: %w", err)
	}
	return &auth.Token{Value: tok.AccessToken, Type: tok.Type(), Expiry: tok.Expiry}, nil
}

// loadStoredCredentials reads and parses the authorized_user file at path.
func loadStoredCredentials(path string) (storedCredentials, error) {
	var creds storedCredentials
	data, err := os.ReadFile(path)
	if err != nil {
		return creds, fmt.Errorf("oauthrenewal: reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return creds, fmt.Errorf("oauthrenewal: parsing %s: %w", path, err)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return creds, fmt.Errorf("oauthrenewal: %s has no client_id/client_secret", path)
	}
	return creds, nil
}
