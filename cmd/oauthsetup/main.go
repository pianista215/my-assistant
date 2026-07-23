// Command oauthsetup is a one-time, interactive tool that turns the OAuth
// desktop client downloaded from Google Cloud Console into a long-lived
// credentials file the server can use unattended.
//
// Run it once, on any machine with a browser (it doesn't need to be the
// VPS). It opens a consent screen, catches the redirect on a local
// loopback listener, and writes an "authorized_user" credentials file
// scoped for Calendar readonly access plus per-file Drive access
// (drive.file) to a single spreadsheet the user picks via the Google
// Picker widget served on that same listener — narrower than granting
// read access to every spreadsheet in the account, and already
// read+write on the picked file so a future menu-writing iteration won't
// need a second re-consent. That file works with internal/calendar's,
// internal/shoppinglist's and internal/weeklymenu's NewClient exactly
// like a service account key would — option.WithCredentialsFile
// auto-detects both formats — so no server code needs to change to use
// it. If the requested scopes ever change, existing users must re-run
// this tool once to re-consent; Google won't silently add scopes to an
// already-issued refresh token.
package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googlecalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

//go:embed picker.html
var pickerHTML string

var pickerTmpl = template.Must(template.New("picker.html").Parse(pickerHTML))

// pickedFile is the spreadsheet the user selected in the Picker widget.
type pickedFile struct {
	ID   string
	Name string
}

func main() {
	_ = godotenv.Load()

	clientJSON := flag.String("client-json", "secrets/oauth-client.json", "path to the OAuth desktop client JSON downloaded from Google Cloud Console")
	out := flag.String("out", "secrets/credentials.json", "where to write the resulting credentials file")
	flag.Parse()

	data, err := os.ReadFile(*clientJSON)
	if err != nil {
		log.Fatalf("reading client JSON: %v", err)
	}

	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_PICKER_API_KEY"))
	if apiKey == "" {
		log.Fatal("GOOGLE_PICKER_API_KEY environment variable is required (see .env.example — create an API key restricted to the Picker API in Google Cloud Console)")
	}

	cfg, err := google.ConfigFromJSON(data, googlecalendar.CalendarReadonlyScope, drive.DriveFileScope)
	if err != nil {
		log.Fatalf("parsing client JSON: %v", err)
	}

	// The Picker widget must be told the Cloud project number via
	// setAppId() for drive.file to actually grant access to the picked
	// file (without it, picking "succeeds" in the browser but the token
	// still has no real access, and later API calls 404). OAuth client
	// IDs are formatted "<project number>-<random>.apps.googleusercontent.com",
	// so it's derived here rather than asked for separately.
	appID, _, ok := strings.Cut(cfg.ClientID, "-")
	if !ok {
		log.Fatalf("could not derive Cloud project number from client ID %q", cfg.ClientID)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("starting local listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	origin := fmt.Sprintf("http://localhost:%d", port)
	cfg.RedirectURL = origin + "/callback"

	state := randomState()
	tokenCh := make(chan *oauth2.Token, 1)
	pickerTokenCh := make(chan *oauth2.Token, 1)
	errCh := make(chan error, 1)
	fileCh := make(chan pickedFile, 1)

	// Each route is registered on its own specific path (not "/"), so an
	// unrelated request the browser sends automatically — e.g. a
	// same-origin GET /favicon.ico once /picker is loaded — 404s instead
	// of being routed to callbackHandler as a bogus, state-less retry.
	mux := http.NewServeMux()
	mux.Handle("/callback", callbackHandler(cfg, state, tokenCh, pickerTokenCh, errCh))
	mux.Handle("/picker", pickerHandler(apiKey, appID, pickerTokenCh, origin))
	mux.Handle("/picker-callback", pickerCallbackHandler(fileCh, errCh))

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Close()

	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	fmt.Println("Open this URL and sign in with the Google account that owns the reference calendar and spreadsheet:")
	fmt.Println(authURL)
	openBrowser(authURL)

	var token *oauth2.Token
	select {
	case token = <-tokenCh:
	case err := <-errCh:
		log.Fatalf("authorization failed: %v", err)
	}

	fmt.Println("Signed in — pick the reference spreadsheet in the browser tab that just opened.")

	var file pickedFile
	select {
	case file = <-fileCh:
	case err := <-errCh:
		log.Fatalf("picker: %v", err)
	}

	creds := struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
		Type         string `json:"type"`
	}{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RefreshToken: token.RefreshToken,
		Type:         "authorized_user",
	}

	f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("writing %s: %v", *out, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(creds); err != nil {
		log.Fatalf("encoding credentials: %v", err)
	}

	fmt.Printf("Wrote %s — point GOOGLE_CREDENTIALS_FILE at this path.\n", *out)

	printCalendars(context.Background(), cfg.Client(context.Background(), token))

	fmt.Printf("\nPicked spreadsheet: %-40s %s\nConfirm this matches GOOGLE_SHEET_ID in your .env.\n", file.ID, file.Name)
}

// printCalendars lists the calendars the just-authorized account can see,
// so the user can pick which one's ID to set as CALENDAR_ID.
func printCalendars(ctx context.Context, httpClient *http.Client) {
	svc, err := googlecalendar.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		log.Printf("warning: could not list calendars: %v", err)
		return
	}
	list, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		log.Printf("warning: could not list calendars: %v", err)
		return
	}

	fmt.Println("\nCalendars available to this account (set CALENDAR_ID to one of these):")
	for _, cal := range list.Items {
		fmt.Printf("  %-40s %s\n", cal.Id, cal.Summary)
	}
}

// callbackHandler handles the single redirect Google sends back to the
// loopback listener after the user completes (or denies) consent,
// exchanges the code for a token, and hands that token to both the
// caller (via tokenCh, for the final calendar listing) and the Picker
// page (via pickerTokenCh) before redirecting the browser there.
func callbackHandler(cfg *oauth2.Config, state string, tokenCh, pickerTokenCh chan<- *oauth2.Token, errCh chan<- error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("state"); got != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch: got %q", got)
			return
		}
		if msg := r.URL.Query().Get("error"); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			errCh <- fmt.Errorf("authorization denied: %s", msg)
			return
		}

		token, err := cfg.Exchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, "token exchange failed", http.StatusInternalServerError)
			errCh <- fmt.Errorf("exchanging code for token: %w", err)
			return
		}
		if token.RefreshToken == "" {
			http.Error(w, "no refresh token returned", http.StatusInternalServerError)
			errCh <- fmt.Errorf("no refresh token returned — revoke prior access at https://myaccount.google.com/permissions and run this again")
			return
		}

		tokenCh <- token
		pickerTokenCh <- token
		http.Redirect(w, r, "/picker", http.StatusFound)
	})
}

// pickerHandler serves the Google Picker widget, restricted to spreadsheets,
// authorized with the access token obtained by callbackHandler.
func pickerHandler(apiKey, appID string, pickerTokenCh <-chan *oauth2.Token, origin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := <-pickerTokenCh
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err := pickerTmpl.Execute(w, struct {
			APIKey      string
			AppID       string
			AccessToken string
			Origin      string
		}{
			APIKey:      apiKey,
			AppID:       appID,
			AccessToken: token.AccessToken,
			Origin:      origin,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// pickerCallbackHandler receives the file the user picked (or a
// cancellation) from picker.html's client-side callback.
func pickerCallbackHandler(fileCh chan<- pickedFile, errCh chan<- error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Cancelled bool   `json:"cancelled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			errCh <- fmt.Errorf("decoding picker callback: %w", err)
			return
		}
		if body.Cancelled {
			errCh <- fmt.Errorf("spreadsheet selection was cancelled")
			return
		}
		fileCh <- pickedFile{ID: body.ID, Name: body.Name}
	})
}

func randomState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("generating state: %v", err)
	}
	return hex.EncodeToString(b)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
