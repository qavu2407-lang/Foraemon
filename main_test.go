package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseRate(t *testing.T) {
	json := `{
		"result": "success",
		"time_last_update_unix": 1664999883,
		"base_code": "AUD",
		"target_code": "VND",
		"conversion_rate": 16543.21
	}`

	quote, err := parseQuote([]byte(json))
	assert.Nil(t, err)
	assert.Equal(t, "success", quote.Result)
	assert.InDelta(t, 16543.21, quote.ConversionRate, 0.001)
}

func Test_ParseRate_RejectsFailure(t *testing.T) {
	// An API error still unmarshals cleanly, so the rate stays zero.
	_, err := parseQuote([]byte(`{"result": "error", "error-type": "invalid-key"}`))
	assert.ErrorContains(t, err, "invalid-key")
}

func Test_CompareRate(t *testing.T) {
	averageRate := 16500.00
	currentRate := 16543.21

	move, pct := computeMove(currentRate, averageRate)

	assert.InDelta(t, 43.21, move, 0.001)
	assert.InDelta(t, 0.2619, pct, 0.0001)
}

func Test_Alert_FlagsALoss(t *testing.T) {
	request := RateRequest{From: "AUD", To: "VND", AverageRate: 16500}
	move, pct := computeMove(16400, request.AverageRate)

	subject, body := alert("AUDVND", request, 16400, move, pct)

	assert.Contains(t, subject, "🔴")
	assert.Contains(t, body, "price is down")
}

func Test_Redact_HidesTheKey(t *testing.T) {
	err := redact(errors.New(`Get "https://v6.exchangerate-api.com/v6/s3cr3t/pair/AUD/VND": timeout`), "s3cr3t")
	assert.NotContains(t, err.Error(), "s3cr3t")
	assert.Contains(t, err.Error(), "REDACTED")
}
