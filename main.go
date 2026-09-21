package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"lorisocchipinti.com/gbp-rates/logger"
	"lorisocchipinti.com/gbp-rates/mailer"
)

const (
	defaultFrom = "AUD"
	defaultTo   = "VND"
)

// The v6 exchangerate-api response. "result" is a status string, not the number.
type Quote struct {
	Result         string  `json:"result"`
	ConversionRate float64 `json:"conversion_rate"`
	ErrorType      string  `json:"error-type"`
}

type RateRequest struct {
	From        string  `json:"from"`
	To          string  `json:"to"`
	AverageRate float64 `json:"avg_rate"`
}

func main() {
	lambda.Start(CheckRate)
}

func CheckRate(ctx context.Context, request RateRequest) error {
	if request.From == "" {
		request.From = defaultFrom
	}
	if request.To == "" {
		request.To = defaultTo
	}
	if request.AverageRate <= 0 {
		return fmt.Errorf("avg_rate must be set to your average %s%s rate", request.From, request.To)
	}

	logger.Log("Fetching rate...")
	currentRate, err := fetchRate(ctx, request.From, request.To)
	if err != nil {
		return err
	}

	move, pct := computeMove(currentRate, request.AverageRate)
	pair := request.From + request.To
	logger.Log(fmt.Sprintf("Current %s rate is %.2f. Possible gain/loss is %+.2f (%+.2f%%)", pair, currentRate, move, pct))

	subject, body := alert(pair, request, currentRate, move, pct)
	logger.Log("Sending alert to user...")
	return mailer.Send(ctx, subject, body)
}

func alert(pair string, request RateRequest, currentRate float64, move float64, pct float64) (string, string) {
	mark, mood := "🟢", "🤑 🤑 🤑"
	headline := "Good news! Price is up"
	if move < 0 {
		mark, mood = "🔴", "😰"
		headline = "No luck, price is down"
	}
	greeting := "Good morning, bee ready to make money today! 🐝 💸 🐝"
	subject := fmt.Sprintf("%s %s %.2f (%+.2f%%)", mark, pair, currentRate, pct)
	body := fmt.Sprintf("%s\n\n%s %s %+.2f %s (%+.2f%%).\n\n1 %s = %.2f %s\nYour average: %.2f\n\n%s",
		greeting,
		mark, headline, move, request.To, pct,
		request.From, currentRate, request.To,
		request.AverageRate, mood)
	return subject, body
}

func fetchRate(ctx context.Context, from string, to string) (float64, error) {
	apikey := os.Getenv("EXCHANGERATE_API_KEY")
	if apikey == "" {
		return 0, fmt.Errorf("EXCHANGERATE_API_KEY is not set")
	}

	// The key travels in the path, so this URL is a secret: never log it.
	url := fmt.Sprintf("https://v6.exchangerate-api.com/v6/%s/pair/%s/%s", apikey, from, to)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, redact(err, apikey)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, redact(err, apikey)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}

	// Errors come back as a normal JSON body with an error-type, so let parseQuote
	// name the actual problem rather than reporting a bare status code.
	quote, err := parseQuote(body)
	if err != nil {
		return 0, fmt.Errorf("rate api returned %s: %w", res.Status, err)
	}
	return quote.ConversionRate, nil
}

// redact keeps the key out of the logs: net/http wraps failures in a *url.Error
// that prints the whole URL, and the key lives in the path.
func redact(err error, apikey string) error {
	return errors.New(strings.ReplaceAll(err.Error(), apikey, "REDACTED"))
}

func parseQuote(data []byte) (Quote, error) {
	var quote Quote
	if err := json.Unmarshal(data, &quote); err != nil {
		return Quote{}, err
	}
	if quote.Result != "success" {
		return Quote{}, fmt.Errorf("lookup failed: %s", quote.ErrorType)
	}
	// A malformed success would otherwise be reported as a 100% crash.
	if quote.ConversionRate <= 0 {
		return Quote{}, fmt.Errorf("no usable conversion_rate")
	}
	return quote, nil
}

// computeMove reports the move in units of the quote currency plus the percentage.
// AUD/VND trades around 18,500, so the forex "pip" convention of 1/10,000 that the
// bot used for EUR-sized pairs is meaningless here.
func computeMove(currentRate float64, averageRate float64) (move float64, pct float64) {
	move = currentRate - averageRate
	return move, move / averageRate * 100
}
