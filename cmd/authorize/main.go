// One-time helper: grants the bot permission to send mail as you and prints the
// REFRESH_TOKEN to paste into .env (and into the Lambda's environment variables).
//
// Usage: set -a; source .env; set +a; go run ./cmd/authorize
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"lorisocchipinti.com/gbp-rates/mailer"
)

const redirectURL = "http://127.0.0.1:8080/callback"

func main() {
	cfg := mailer.Config()
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		fmt.Println("CLIENT_ID and CLIENT_SECRET must be set (see .env.example)")
		os.Exit(1)
	}
	cfg.RedirectURL = redirectURL

	// AccessTypeOffline gets a refresh token; ApprovalForce makes Google hand one
	// over again on re-runs instead of silently omitting it.
	fmt.Println("Open this URL in your browser:")
	fmt.Println(cfg.AuthCodeURL("", oauth2.AccessTypeOffline, oauth2.ApprovalForce))

	codes := make(chan string, 1)
	srv := &http.Server{Addr: "127.0.0.1:8080"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if e := r.URL.Query().Get("error"); e != "" {
			http.Error(w, e, http.StatusBadRequest)
			codes <- ""
			return
		}
		fmt.Fprintln(w, "Done, you can close this tab.")
		codes <- r.URL.Query().Get("code")
	})
	go srv.ListenAndServe()
	defer srv.Close()

	code := <-codes
	if code == "" {
		fmt.Println("no authorization code received")
		os.Exit(1)
	}

	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if tok.RefreshToken == "" {
		fmt.Println("Google returned no refresh token; revoke the app at https://myaccount.google.com/permissions and retry")
		os.Exit(1)
	}
	fmt.Printf("\nREFRESH_TOKEN=%s\n", tok.RefreshToken)
}
