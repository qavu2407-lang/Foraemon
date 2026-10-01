// Package tools fetches the outside evidence for the email: news headlines, the economic
// calendar and the SBV central rate. Every source is keyless and a soft dependency: the
// caller sends the email without whatever fails.
package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 30 * time.Second}

// get returns the body of a 200 response. Some feeds refuse Go's default User-Agent.
func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; forex-bot)")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 10<<20))
}
