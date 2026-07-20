// Command oauthsetup is a one-time, interactive tool that turns the OAuth
// desktop client downloaded from Google Cloud Console into a long-lived
// credentials file the server can use unattended.
//
// Run it once, on any machine with a browser (it doesn't need to be the
// VPS). It opens a consent screen, catches the redirect on a local
// loopback listener, and writes an "authorized_user" credentials file
// scoped for both Calendar and Sheets readonly access. That file works
// with internal/calendar's and internal/shoppinglist's NewClient exactly
// like a service account key would — option.WithCredentialsFile
// auto-detects both formats — so no server code needs to change to use
// it. If the requested scopes ever change, existing users must re-run
// this tool once to re-consent; Google won't silently add scopes to an
// already-issued refresh token.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googlecalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func main() {
	clientJSON := flag.String("client-json", "secrets/oauth-client.json", "path to the OAuth desktop client JSON downloaded from Google Cloud Console")
	out := flag.String("out", "secrets/credentials.json", "where to write the resulting credentials file")
	flag.Parse()

	data, err := os.ReadFile(*clientJSON)
	if err != nil {
		log.Fatalf("reading client JSON: %v", err)
	}
	cfg, err := google.ConfigFromJSON(data, googlecalendar.CalendarReadonlyScope, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		log.Fatalf("parsing client JSON: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("starting local listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://localhost:%d", port)

	state := randomState()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	server := &http.Server{Handler: callbackHandler(state, codeCh, errCh)}
	go server.Serve(listener)
	defer server.Close()

	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	fmt.Println("Open this URL and sign in with the Google account that owns the reference calendar:")
	fmt.Println(authURL)
	openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		log.Fatalf("authorization failed: %v", err)
	}

	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("exchanging code for token: %v", err)
	}
	if token.RefreshToken == "" {
		log.Fatal("no refresh token returned — revoke prior access at https://myaccount.google.com/permissions and run this again")
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
// loopback listener after the user completes (or denies) consent.
func callbackHandler(state string, codeCh chan<- string, errCh chan<- error) http.Handler {
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
		fmt.Fprintln(w, "Authorized — you can close this tab.")
		codeCh <- r.URL.Query().Get("code")
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
