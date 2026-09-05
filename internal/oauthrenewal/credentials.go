package oauthrenewal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	googleauth "cloud.google.com/go/auth"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

// storedCredentials is the authorized_user-format JSON shape written by
// cmd/oauthsetup and cmd/deviceauthsetup, and consumed by
// option.WithCredentialsFile in internal/calendar, internal/shoppinglist
// and internal/weeklymenu — mirrors cmd/oauthsetup/main.go's own creds
// struct exactly, so both tools (and this package) write/read the same
// shape.
type storedCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// ClientCredentials reads the client_id/client_secret embedded in the
// credentials file at path (the same file GOOGLE_CREDENTIALS_FILE
// points at). After the one-time cmd/deviceauthsetup bootstrap, that
// file's client is the "TVs and Limited Input devices" OAuth client the
// device flow requires — so no separate client_id/client_secret
// configuration is needed for renewal, only for that initial bootstrap.
func ClientCredentials(path string) (clientID, clientSecret string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("oauthrenewal: reading %s: %w", path, err)
	}
	var creds storedCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", "", fmt.Errorf("oauthrenewal: parsing %s: %w", path, err)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return "", "", fmt.Errorf("oauthrenewal: %s has no client_id/client_secret", path)
	}
	return creds.ClientID, creds.ClientSecret, nil
}

// SaveCredentials writes path in the same authorized_user shape
// cmd/oauthsetup writes (2-space-indented JSON, 0600 permissions), so
// option.WithCredentialsFile keeps working unchanged for
// internal/calendar, internal/shoppinglist and internal/weeklymenu once
// they're reconstructed against the renewed file (see
// internal/server.Server's client swap).
func SaveCredentials(path, clientID, clientSecret string, token *oauth2.Token) error {
	creds := storedCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: token.RefreshToken,
		Type:         "authorized_user",
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("oauthrenewal: writing %s: %w", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(creds)
}

// IsReauthError reports whether err is one of the shapes a dead or
// revoked refresh token can produce, as opposed to a transient
// network/API error that doesn't call for replacing the display with the
// reauthorization screen. All three were found empirically, by live
// testing against the real API rather than assumed upfront — see the
// "IsReauthError checks ... error shapes, found via live testing" bullet
// in CLAUDE.md for the full story of how each one surfaced:
//
//   - A dead credential caught when a refresh actually happens, via the
//     newer `cloud.google.com/go/auth` library that `google.golang.org/api`
//     v0.289.0 (option.WithCredentialsFile) uses internally for this —
//     *googleauth.Error, whose Error() renders as `auth: "invalid_grant"
//     "..."`. Its `code` field mirroring that isn't exported, so
//     tokenErrorCode below decodes the same raw JSON body (exported as
//     Error.Body) the library itself already parsed to get it.
//   - The same failure caught by the older golang.org/x/oauth2 path
//     instead (*oauth2.RetrieveError, `oauth2: "invalid_grant" "..."`) —
//     kept as a defensive fallback in case a different code path (or a
//     future dependency version) ever takes this route instead; not
//     observed in this app's own testing, since calendar/shoppinglist/
//     weeklymenu all go through the newer library above.
//   - A plain 401 from the Calendar/Sheets API itself (*googleapi.Error)
//     — happens when the cached access token *hasn't* expired yet by its
//     own clock, so the transport sends it straight to the API without
//     ever attempting a refresh; if the underlying grant died (revoked,
//     or the natural 7-day Testing-mode cutoff) in the meantime, Google
//     rejects that access token at the API call itself instead.
func IsReauthError(err error) bool {
	var authErr *googleauth.Error
	if errors.As(err, &authErr) && tokenErrorCode(authErr.Body) == "invalid_grant" {
		return true
	}
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
		return true
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusUnauthorized {
		return true
	}
	return false
}

// tokenErrorCode extracts the "error" field from a raw OAuth2 token
// endpoint error response body (RFC 6749 §5.2's JSON shape) — the same
// bytes cloud.google.com/go/auth's *Error.Body carries and already
// parsed into its own unexported `code` field, decoded again here since
// that field has no exported accessor. A body that isn't this JSON shape
// (or empty) just yields an empty string, never an error of its own.
func tokenErrorCode(body []byte) string {
	var parsed struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	return parsed.Error
}
