// Command deviceauthsetup is a one-time, interactive tool that bootstraps
// the "TVs and Limited Input devices" OAuth client internal/oauthrenewal's
// Device Authorization Grant flow needs, writing (replacing) the
// credentials file with this new client's own client_id/client_secret
// and a fresh refresh token obtained through it.
//
// Why a second OAuth client at all: Google's device-code endpoint rejects
// any client that isn't of this exact type (invalid_client) — the
// Desktop-type client cmd/oauthsetup uses can't be reused for this. Why
// bootstrap it interactively here, at a PC, rather than letting
// internal/server obtain it unattended the very first time: the device
// flow's own consent step still needs a real human to open a browser and
// approve it once per OAuth client — exactly like cmd/oauthsetup's own
// first run. internal/server only ever *renews* an already-consented
// client afterward (see that package's doc comment); it never performs
// this initial consent itself.
//
// Run it once, right after creating the new client in Google Cloud
// Console (APIs & Services -> Credentials -> Create credentials -> OAuth
// client ID -> Application type: "TVs and Limited Input devices"), same
// project as the existing Desktop client — no publishing/verification
// change needed, since it reuses the same OAuth consent screen. Unlike
// cmd/oauthsetup, no local HTTP listener catches a redirect: the device
// flow has no redirect_uri at all. This tool prints the verification
// URL/code (and opens it in this machine's own browser as a convenience —
// on the real server this same step becomes a QR code on the e-ink
// panel, see internal/display.NewReauthScreen) and blocks polling Google
// until it's approved.
//
// Because drive.file grants access per (file, OAuth client) pair rather
// than per (file, account), this new client isn't guaranteed to already
// have access to the shopping-list/weekly-menu spreadsheet the original
// cmd/oauthsetup run picked, even though it's the same Google account.
// This tool checks that directly after obtaining the token and, only if
// the check fails, launches the same Picker flow cmd/oauthsetup uses
// (via internal/oauthpicker) to re-grant it — a one-time step here too,
// not something that recurs on every renewal.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/joho/godotenv"

	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/oauthpicker"
	"github.com/pianista215/my-assistant/internal/oauthrenewal"
)

func main() {
	_ = godotenv.Load()

	clientID := flag.String("client-id", "", `Client ID of the "TVs and Limited Input devices" OAuth client (Google Cloud Console -> APIs & Services -> Credentials)`)
	clientSecret := flag.String("client-secret", "", "Client secret of that same OAuth client")
	credentialsFile := flag.String("credentials-file", "", "Path to write the resulting credentials file — defaults to GOOGLE_CREDENTIALS_FILE from the environment/.env")
	sheetID := flag.String("sheet-id", "", "Spreadsheet ID to verify this client can read after obtaining the token — defaults to GOOGLE_SHEET_ID from the environment/.env")
	pickerAPIKey := flag.String("picker-api-key", "", "Picker API key, only used if the spreadsheet needs to be re-picked — defaults to GOOGLE_PICKER_API_KEY from the environment/.env")
	flag.Parse()

	if *clientID == "" || *clientSecret == "" {
		log.Fatal(`both --client-id and --client-secret are required (from the "TVs and Limited Input devices" OAuth client)`)
	}

	cfg, cfgErr := config.Load()
	out := *credentialsFile
	if out == "" {
		if cfgErr != nil {
			log.Fatalf("--credentials-file not given and reading it from config failed: %v", cfgErr)
		}
		out = cfg.GoogleCredentialsFile
	}
	sheet := *sheetID
	if sheet == "" {
		if cfgErr != nil {
			log.Fatalf("--sheet-id not given and reading it from config failed: %v", cfgErr)
		}
		sheet = cfg.GoogleSheetID
	}

	ctx := context.Background()
	pending, err := oauthrenewal.StartDeviceAuthWithEndpoint(ctx, *clientID, *clientSecret, google.Endpoint)
	if err != nil {
		log.Fatalf("starting device authorization: %v", err)
	}

	fmt.Println("Open this URL on any device with a browser (opened automatically here as a convenience — on the real server this step is a QR code on the e-ink panel instead):")
	fmt.Println(pending.VerificationURL)
	if pending.NeedsUserCode {
		fmt.Printf("Enter this code when prompted: %s\n", pending.UserCode)
	}
	openBrowser(pending.VerificationURL)

	fmt.Println("Waiting for approval...")
	token, err := pending.WaitForToken(ctx)
	if err != nil {
		log.Fatalf("device authorization: %v", err)
	}
	fmt.Println("Approved.")

	if err := oauthrenewal.SaveCredentials(out, *clientID, *clientSecret, token); err != nil {
		log.Fatalf("writing %s: %v", out, err)
	}
	fmt.Printf("Wrote %s.\n", out)

	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	if checkSheetAccess(ctx, httpClient, sheet) {
		fmt.Println("Confirmed: this client can already read the configured spreadsheet — nothing else to do.")
		return
	}

	fmt.Println("This client cannot read the configured spreadsheet yet (drive.file access is granted per OAuth client, not per account) — picking it again to grant access.")
	apiKey := *pickerAPIKey
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("GOOGLE_PICKER_API_KEY"))
	}
	if apiKey == "" {
		log.Fatal("no Picker API key available (--picker-api-key or GOOGLE_PICKER_API_KEY) — cannot re-pick the spreadsheet")
	}
	rePickSpreadsheet(*clientID, apiKey, token)
}

// checkSheetAccess reports whether httpClient (authorized as the new
// device-flow client) can read sheetID's metadata — the cheapest possible
// probe that this new client's drive.file grant actually covers the same
// spreadsheet cmd/oauthsetup's client already had access to.
func checkSheetAccess(ctx context.Context, httpClient *http.Client, sheetID string) bool {
	svc, err := sheets.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return false
	}
	_, err = svc.Spreadsheets.Get(sheetID).Fields("properties.title").Context(ctx).Do()
	return err == nil
}

// rePickSpreadsheet launches the same Picker widget cmd/oauthsetup uses
// (internal/oauthpicker) so the user can re-select the reference
// spreadsheet, granting this new OAuth client its own drive.file access
// to it.
func rePickSpreadsheet(clientID, apiKey string, token *oauth2.Token) {
	// The Picker widget needs the Cloud project number via setAppId() —
	// see cmd/oauthsetup/main.go's identical derivation for why this
	// works and why it's not asked for separately.
	appID, _, ok := strings.Cut(clientID, "-")
	if !ok {
		log.Fatalf("could not derive Cloud project number from client ID %q", clientID)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("starting local listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	origin := fmt.Sprintf("http://localhost:%d", port)

	fileCh := make(chan oauthpicker.PickedFile, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.Handle("/picker", oauthpicker.Handler(apiKey, appID, token.AccessToken, origin))
	mux.Handle("/picker-callback", oauthpicker.CallbackHandler(fileCh, errCh))

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Close()

	pickerURL := origin + "/picker"
	fmt.Println("Opening the spreadsheet picker:")
	fmt.Println(pickerURL)
	openBrowser(pickerURL)

	select {
	case file := <-fileCh:
		fmt.Printf("Picked %s (%s) — access granted to the new client.\n", file.Name, file.ID)
	case err := <-errCh:
		log.Fatalf("picker: %v", err)
	}
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
