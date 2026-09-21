# Forex Bot

A basic bot to fetch currency rates and email you about any potential gain/loss.
Set up here for **AUD/VND**.

## Getting Started

### 1. Rate API

I use [exchangerate-api.com](https://app.exchangerate-api.com/) because it's free and
simple, but anything goes as long as you are willing to adapt the code a bit. Put the
key in `EXCHANGERATE_API_KEY`.

The key travels in the URL path for this API, so `fetchRate` never logs the URL and
scrubs the key out of network errors before they reach CloudWatch.

### 2. Gmail

1. In the [Google Cloud Console](https://console.cloud.google.com/), enable the Gmail API
   and create an OAuth client of type **Desktop app**. Put the id and secret in `CLIENT_ID`
   and `CLIENT_SECRET`.
1. While the app is in "Testing", add your own address as a test user.
1. Mint a refresh token once:
   ```sh
   cp .env.example .env   # fill in the values you have so far
   set -a; source .env; set +a
   go run ./cmd/authorize
   ```
   Open the printed URL, approve, and paste the resulting `REFRESH_TOKEN` into `.env`.
1. Set `MAIL_TO` to wherever the alerts should land.

Note: a refresh token for an app still in "Testing" expires after 7 days. Publish the
app (no verification needed for your own account with the `gmail.send` scope) to stop
re-running the authorize step.

### 3. Deploy

This bot runs on AWS Lambda, so it's necessary to create a zip archive (sigh) to
deploy the code.

1. Run `./zip.sh` to generate the archive.
1. Create the function on the **`provided.al2023`** runtime with handler `bootstrap`
   (the old `go1.x` runtime is retired).
1. Copy every variable from your `.env` into the function's environment variables —
   `.env` itself is only used for local runs.
1. Set the input data. `from`/`to` default to AUD/VND if omitted, but `avg_rate` is
   required and is the average rate you bought at:
    ```json
    {
        "from": "AUD",
        "to": "VND",
        "avg_rate": 18500.00
    }
    ```
1. Schedule it for **09:00 Asia/Ho_Chi_Minh**. EventBridge cron is UTC and ICT is
   UTC+7 with no daylight saving, so that is 02:00 UTC:
    ```
    cron(0 2 * * ? *)
    ```
   One run, one email. The handler has no clock logic of its own — change the
   schedule, not the code, if you want a different time.
1. ???
1. Profit!

Gains and losses are reported in whole VND per 1 AUD plus a percentage, not in PIPs —
AUD/VND trades around 18,500, so the 1/10,000 PIP convention doesn't mean anything for
this pair. Each run sends exactly one mail, greeting and rate together.

## Author

Loris Occhipinti
* ✉️Contact me at: loris@lorisocchipinti.com
* ⭐Website: https://blog.lorisocchipinti.com
