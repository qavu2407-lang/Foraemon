# Forex Bot

A daily email reporting AUD/VND, AUD/USD, USD/VND and CZK/VND, with the % change since
the previous publication and the AUD/VND move split into its two causes: the Australian
dollar against the US dollar, and the dong against the US dollar.

No LLM is involved. Every number is computed in Go, and every word of the email lives in
[templates/email.tmpl](templates/email.tmpl). See [PLAN.md](PLAN.md) for the design and
the reasoning behind it.

## Getting Started

### 1. Rate API

I use [exchangerate-api.com](https://app.exchangerate-api.com/) because it's free and
simple, but anything goes as long as you are willing to adapt the code a bit. Put the
key in `EXCHANGERATE_API_KEY`.

One call to `/latest/USD` supplies all four pairs, so they always come from the same
publication. The key travels in the URL path for this API, so `fetchRates` never logs the
URL and scrubs the key out of network errors before they reach CloudWatch.

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
1. Set `MAIL_TO` to wherever the alerts should land. Comma-separate the value to send
   to several people: `MAIL_TO=you@example.com,someone@example.com`. They all appear
   on the same `To:` header, so everyone sees who else got it.

Note: a refresh token for an app still in "Testing" expires after 7 days. Publish the
app (no verification needed for your own account with the `gmail.send` scope) to stop
re-running the authorize step.

### 3. Rate history

"% change" means change since the previous publication, the convention rate platforms use.
That needs yesterday's rates, and Lambda keeps nothing between runs, so each run appends one
snapshot to a small JSON file (~31 KB a year).

- **Locally**, it's `./history.json` (gitignored), or wherever `HISTORY_FILE` points.
- **On Lambda**, create an S3 bucket, set `HISTORY_BUCKET` to its name, and give the
  function's role this policy:
  ```json
  {
      "Version": "2012-10-17",
      "Statement": [{
          "Effect": "Allow",
          "Action": ["s3:GetObject", "s3:PutObject"],
          "Resource": "arn:aws:s3:::YOUR-BUCKET/history.json"
      }, {
          "Effect": "Allow",
          "Action": "s3:ListBucket",
          "Resource": "arn:aws:s3:::YOUR-BUCKET"
      }]
  }
  ```
  `ListBucket` is what lets S3 answer "not found" on the first run instead of "access
  denied".

The first run has nothing to compare against and says so; change figures appear from the
second publication onwards. If the history can't be read, the email still goes out without
change figures, and the history is left untouched rather than overwritten.

### 4. Deploy

This bot runs on AWS Lambda, so it's necessary to create a zip archive (sigh) to
deploy the code.

1. Run `./zip.sh` to generate the archive.
1. Create the function on the **`provided.al2023`** runtime with handler `bootstrap`
   (the old `go1.x` runtime is retired).
1. Copy every variable from your `.env` into the function's environment variables —
   `.env` itself is only used for local runs.
1. Set the input data. It's optional: `{}` works. To also see how far each pair is from
   where you converted, add an `entry` per pair:
    ```json
    {
        "entry": {
            "AUD/VND": 18450.00
        }
    }
    ```
   **Use the mid-market rate on the day you converted**, not the rate you actually got.
   What you received had a fee taken out (~0.4–2% on Wise), which is several times a
   typical daily move and would show up as a permanent phantom gain. The old
   `from`/`to`/`avg_rate` fields are gone and ignored if present.
1. Schedule it for **09:00 Asia/Ho_Chi_Minh**. EventBridge cron is UTC and ICT is
   UTC+7 with no daylight saving, so that is 02:00 UTC:
    ```
    cron(0 2 * * ? *)
    ```
   One run, one email. The handler has no clock logic of its own — change the
   schedule, not the code, if you want a different time.
1. ???
1. Profit!

## What the email says

```
AUD/VND   18,238.19   -1.38%
  AUD leg  -1.32pp  AUD weaker vs USD
  VND leg  -0.06pp  VND stronger vs USD
         -1.15% vs your entry 18,450.00
```

- **%** is a pair's own change since the previous publication.
- **pp** is how much of the AUD/VND change each leg accounts for. The two legs always add
  up to the headline figure.
- The VND leg is USD/VND, so a **positive** VND leg means the dong got **weaker**. The
  words next to each leg say which way it went; trust those over the sign.
- All rates are mid-market. What you receive on a transfer is lower by the provider's fee.

## Author

Loris Occhipinti
* ✉️Contact me at: loris@lorisocchipinti.com
* ⭐Website: https://blog.lorisocchipinti.com
