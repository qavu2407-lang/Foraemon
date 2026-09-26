package mailer

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Config builds the OAuth2 config from the environment. Shared with cmd/authorize,
// which is what mints the REFRESH_TOKEN in the first place.
func Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
		Scopes: []string{gmail.GmailSendScope},
	}
}

// recipients normalises the MAIL_TO list. It takes one address or several separated
// by commas; a trailing comma or stray space would otherwise build a header Gmail
// rejects, costing that run's mail.
func recipients(raw string) string {
	var out []string
	for _, addr := range strings.Split(raw, ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			out = append(out, addr)
		}
	}
	return strings.Join(out, ", ")
}

// Send delivers a plain-text mail to every address in MAIL_TO, as the account that granted REFRESH_TOKEN.
func Send(ctx context.Context, subject string, body string) error {
	to := recipients(os.Getenv("MAIL_TO"))
	if to == "" {
		return fmt.Errorf("MAIL_TO is not set")
	}
	refresh := os.Getenv("REFRESH_TOKEN")
	if refresh == "" {
		return fmt.Errorf("REFRESH_TOKEN is not set, run: go run ./cmd/authorize")
	}

	src := Config().TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	srv, err := gmail.NewService(ctx, option.WithTokenSource(src))
	if err != nil {
		return err
	}

	// Subjects carry emoji, so they need RFC 2047 encoding to survive the header.
	raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		to, mime.QEncoding.Encode("utf-8", subject), body)

	_, err = srv.Users.Messages.Send("me", &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString([]byte(raw)),
	}).Context(ctx).Do()
	return err
}
