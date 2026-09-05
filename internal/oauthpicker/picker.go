// Package oauthpicker serves the Google Picker widget used to let the
// user choose the reference spreadsheet during an interactive OAuth setup
// tool — factored out of cmd/oauthsetup once cmd/deviceauthsetup needed
// the exact same picker page and callback handling, rather than
// duplicating picker.html and its two handlers across both commands.
//
// This package only renders the picker page and decodes what it reports
// back — it doesn't register routes or know anything about how its
// caller obtained the OAuth access token it's given (cmd/oauthsetup's
// loopback-redirect flow and cmd/deviceauthsetup's device flow obtain one
// very differently), and it doesn't run an HTTP server itself.
package oauthpicker

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed picker.html
var pickerHTML string

var pickerTmpl = template.Must(template.New("picker.html").Parse(pickerHTML))

// PickedFile is the spreadsheet the user selected in the Picker widget.
type PickedFile struct {
	ID   string
	Name string
}

// Handler serves the Picker widget page (restricted to spreadsheets),
// authorized with accessToken. The caller registers it at whatever path
// it likes (cmd/oauthsetup and cmd/deviceauthsetup both use "/picker") —
// this package doesn't register routes itself.
func Handler(apiKey, appID, accessToken, origin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err := pickerTmpl.Execute(w, struct {
			APIKey      string
			AppID       string
			AccessToken string
			Origin      string
		}{
			APIKey:      apiKey,
			AppID:       appID,
			AccessToken: accessToken,
			Origin:      origin,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// CallbackHandler receives the file the user picked (or a cancellation)
// from picker.html's client-side callback: a successful pick is sent on
// fileCh, a cancellation or decode failure on errCh.
func CallbackHandler(fileCh chan<- PickedFile, errCh chan<- error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Cancelled bool   `json:"cancelled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			errCh <- fmt.Errorf("oauthpicker: decoding picker callback: %w", err)
			return
		}
		if body.Cancelled {
			errCh <- fmt.Errorf("oauthpicker: spreadsheet selection was cancelled")
			return
		}
		fileCh <- PickedFile{ID: body.ID, Name: body.Name}
	})
}
