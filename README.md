# Foraemon: A Forex Bot that automatically fetches, explains – and augments Human Intelligence

A daily email reporting currency rates based on personal use. I chose AUD/VND, AUD/USD, USD/VND, CZK/USD, and CZK/VND.

This project runs in three phases, depending on different development stages and use cases. 

1. Phase 1: Automatically fetching rates, calculating simple % rate changes and sending emails only.
    -  No LLM is involved. Every number is computed in Go, and every word of the email lives in
[templates/email.tmpl](templates/email.tmpl).
    - Hardcoded, reliable but limited use.
2. Phase 2: Automatically fetching news sources and explaining the changes.
    - LLM is involved for synthesis and reasoning. Possible agentic loop involved for dynamic news fetch based on layer.
    - Design to be updated in later versions.
3. Phase 3. Human Autonomy Layer
    - A governance layer will be added to ensure humans TRULY learns the signals and make sound judgments on their own. 
    - Design to be updated in later versions.

Based on [Ipanov7/forex-bot](https://github.com/Ipanov7/forex-bot) by Loris Occhipinti.

## Architecture: Foraemon V1

### Overview: Phase 1

 Once a day, a trigger runs the bot. The bot fetches one set of exchange rates, compares
them with the previous day's, and emails the result. It then exits. Nothing runs between
emails.

```
  cron-job.org (your time)                      EventBridge cron
        │  POST workflow_dispatch                     │
        ▼                                             ▼
  GitHub Actions: go run .                      AWS Lambda: bootstrap
  history.json in the Actions cache             history.json in S3
        └──────────────────────┬──────────────────────┘
                               ▼
                    CheckRate(ctx, request)                    main.go
                               │
       ┌───────────────────────┼─────────────────────────────┐
       │ 1. validateEntries    │ reject unknown pairs / rates ≤ 0
       │ 2. loadHistory        │ previous snapshots          history.go
       │ 3. fetchRates         │ one GET /latest/USD         rates.go  
       │ 4. derivePairs        │ five pairs + previous       rates.go 
       │ 5. saveHistory        │ append today's snapshot     history.go
       │ 6. buildView, render  │ numbers → text              templates.go 
       │ 7. mailer.Send        │ plain-text mail             mailer/   
       └───────────────────────┴─────────────────────────────┘
```

The same binary runs in both places. `main()` checks for `AWS_LAMBDA_RUNTIME_API`. If it is set, the binary runs as a Lambda handler. If not, it runs once and exits, and the optional event is read from the `EVENT` environment variable. The trigger decides when the email goes out. The code has no clock logic.

### Files

| File | Responsibility |
|---|---|
| [main.go](main.go) | Entry point. Runs the steps in order and decides what counts as fatal. |
| [rates.go](rates.go) | Fetches and parses rates, derives the pairs, splits the AUD/VND and CZK/VND moves, formats numbers. |
| [history.go](history.go) | Reads and writes `history.json`, from S3 or a local file. Finds the previous publication. |
| [templates.go](templates.go) | Turns the numbers into display strings and renders the embedded template. |
| [templates/email.tmpl](templates/email.tmpl) | Every word of the email: the `subject`, `body`, `line`, `rate`, `legs` and `entry` blocks. |
| [mailer/](mailer/mailer.go) | Sends mail through the Gmail API using an OAuth refresh token. Cleans up `MAIL_TO`. |
| [logger/](logger/logger.go) | Timestamped stdout, which ends up in CloudWatch or the Actions log. |
| [cmd/authorize/](cmd/authorize/main.go) | One-time local helper that creates the Gmail `REFRESH_TOKEN`. |
| [.github/workflows/daily-email.yml](.github/workflows/daily-email.yml) | The GitHub Actions job. |

Everything is in one flat `package main` apart from `mailer` and `logger`. There's one entry point and data flows in one direction, so there are no boundaries worth enforcing with more packages.

### Rates: one call, five pairs

The provider quotes every currency per 1 USD. Here, I used **mid-market** rate from Exchange-rate API because it is free for my current usage. A single request to `/latest/USD` supplies all of them, and the five pairs are derived from it:

| Pair | Derived as | Decimals shown |
|---|---|---|
| **AUD/VND** (headline) | VND ÷ AUD | 4 |
| AUD/USD | 1 ÷ AUD | 4 |
| USD/VND | VND (read directly) | 0 |
| CZK/USD | 1 ÷ CZK | 4 |
| CZK/VND | VND ÷ CZK | 4 |

- **Why one call:** all five pairs come from the same publication, with one timestamp. Separate calls could each land on a different rate update. The AUD/VND split would then describe a move that never happened at any single moment.
- **Direction convention:** every rate means "units of QUOTE per 1 BASE" and is named `BASE/QUOTE`. Reciprocals are taken only in `pairDefs` in [rates.go](rates.go). Mixing directions is the classic currency bug, and both versions look like plausible numbers.
- **Strict parsing:** `parseRates` keeps only AUD, VND and CZK. It fails if any of them is missing or not positive, or if the response has no publication time. If the provider changes its format, the bot stops instead of dividing by zero.
- **Key hygiene:** the API key is part of the URL path. The URL is never logged, and `redact` removes the key from network errors.

### "% change": what it's measured against

"% change" is measured against the **previous publication**, which is what rate sites such as Wise and XE show. Separately, you can supply an `entry` rate per pair. That adds a line labelled "vs your entry", which compares today's rate with the rate on the day you converted. The two figures answer different questions, so they get different labels.

### Splitting the AUD/VND and CZK/VND moves

```
AUD/VND = AUD/USD × USD/VND
          AUD leg   VND leg
```

```
CZK/VND = CZK/USD × USD/VND
          CZK leg   VND leg
```

If AUD/VND moved, either the Australian dollar moved against the US dollar, or the dong did, or both. CZK/VND works the same way with the Czech koruna. `attribute` in [rates.go](rates.go) splits each move between its two legs, and each leg is a pair the email also shows:

- **Log contributions.** Percentages of a product don't add up exactly, because of a small cross term. Logs do. Each leg's share of the log move is scaled to the plain percentage, so the two legs sum to the headline figure with nothing left over.
- **Flat guard.** If the total move would print as 0.00%, the split would divide by almost zero. The email says "flat" instead.
- **Round once, at the end.** Calculations use full `float64`. When rounding to 0.01, the larger leg absorbs any leftover so the column still adds up. A table that doesn't add up makes every number in it look wrong.
- **Words beside every sign.** The VND leg is USD/VND, so a *positive* VND leg means the dong got *weaker*. The template prints the direction in words, because the sign alone is easy to misread.
- **Units:** `%` is a pair's own change. `pp` is how much of the AUD/VND or CZK/VND change one leg accounts for.

### History

Each run appends one snapshot (publication time plus the three per-USD rates) to
`history.json`. That's about 85 bytes a day, or ~31 KB a year, capped at 400 snapshots.

| Where it runs | Where history lives |
|---|---|
| Lambda | S3 object `history.json` in `HISTORY_BUCKET` |
| GitHub Actions | `history.json` in the Actions cache, saved under a new key each run and restored from the newest |
| Locally | `./history.json`, or `HISTORY_FILE` |

- **Duplicate runs:** the free tier publishes about once a day, so a second run can see a
  publication that's already stored. `appendSnapshot` skips it, and `previous` compares
  against the last *older* publication, never against itself.
- **Why one JSON file and not a database:** ten years of data fits in ~300 KB, and the
  only query is "the previous entry". A database would add a driver, a schema and
  credentials. On AWS, RDS would also need a VPC and a NAT gateway (~$45/month) to
  store 31 KB a year.
- **Not yet tested:** the S3 path has only been compiled, never run against a real bucket.

### Templates

[templates/email.tmpl](templates/email.tmpl) is compiled into the binary with `//go:embed`
and parsed at start-up with `text/template`. If the template is broken, the program fails
before anything is sent, not halfway through. Go passes it formatted strings and
directions (`+1`, `0`, `-1`). The template decides the wording. Changing the copy means
editing that file and redeploying. No code changes are needed.

The email is plain text, aligned in columns with spaces. It looks the same in every mail
client, including in dark mode.

### Failure policy

The rates are the whole point of the email, so anything that would make them wrong stops
the run. Anything that only affects the change figures does not.

| What fails | What happens |
|---|---|
| `entry` names an unknown pair, or has a rate ≤ 0 | Stops before any network call |
| Rate API is unreachable, returns an error, or is missing a currency | Stops. No email. |
| History can't be read | The email is sent without change figures. The history file is **left untouched** rather than overwritten with a single day. |
| History can't be saved | Logged. The email is still sent. |
| Move too small to split | The email prints "flat". This is a normal outcome, not an error. |
| Template is broken | Fails at start-up |
| Gmail send fails | The run fails (a Lambda error, or a red Actions run) |

There are no retries. With one run a day, a failed run is followed by tomorrow's normal
email.

### Configuration

| Environment variables (secrets) | Event (optional) |
|---|---|
| `EXCHANGERATE_API_KEY` | `{"entry": {"AUD/VND": 18450}}` |
| `CLIENT_ID`, `CLIENT_SECRET`, `REFRESH_TOKEN` | Lambda: the EventBridge input |
| `MAIL_TO` (comma-separated) | Actions/local: the `EVENT` variable |
| `HISTORY_BUCKET` or `HISTORY_FILE` | |

Secrets never go in the event. Event bodies show up in schedule configs and logs.

### Testing

[main_test.go](main_test.go) covers the pure functions without any network calls: rate
parsing, the direction convention, attribution (same direction, opposite legs, one leg
still, near-zero), rounded legs adding up, history dedupe and cap, entry validation,
number formatting, key redaction, and a golden test that fixes the whole rendered email.
[mailer/mailer_test.go](mailer/mailer_test.go) covers `MAIL_TO` parsing. The network
wrappers contain no logic, so nothing mocks HTTP.

```sh
go vet ./... && go test ./...
```

## Getting Started

### 1. Rate API

I use [exchangerate-api.com](https://app.exchangerate-api.com/) because it's free and
simple, but anything goes as long as you are willing to adapt the code a bit. Put the
key in `EXCHANGERATE_API_KEY`.

One call to `/latest/USD` supplies all five pairs, so they always come from the same
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

### 4b. Or: GitHub Actions instead of Lambda

[.github/workflows/daily-email.yml](.github/workflows/daily-email.yml) sends the email,
with no AWS account needed. [cron-job.org](https://cron-job.org) starts it at the time
you choose. GitHub's own `schedule:` trigger can start runs late or skip them.

1. Under **Settings > Secrets and variables > Actions**, add the secrets `CLIENT_ID`,
   `CLIENT_SECRET`, `REFRESH_TOKEN`, `MAIL_TO` and `EXCHANGERATE_API_KEY`.
1. Optionally, add a **variable** `EVENT` holding the same JSON as the Lambda input,
   e.g. `{"entry":{"AUD/VND":18450}}`.
1. Merge to the default branch. Test it with **Actions > Daily email > Run workflow**.
1. Create a [fine-grained token](https://github.com/settings/personal-access-tokens/new):
   **Only select repositories** (this one), permission **Actions: Read and write**,
   nothing else.
1. On cron-job.org, create a job:
   - **URL:** `https://api.github.com/repos/OWNER/REPO/actions/workflows/daily-email.yml/dispatches`
   - **Schedule:** custom, every day at your time, in your timezone (e.g. 05:00 `Asia/Ho_Chi_Minh`).
     cron-job.org handles the timezone, so there's no UTC conversion.
   - **Advanced > Request method:** `POST`
   - **Advanced > Headers:**
     `Authorization: Bearer <token>`, `Accept: application/vnd.github+json`,
     `X-GitHub-Api-Version: 2022-11-28`, `Content-Type: application/json`
   - **Advanced > Request body:** `{"ref":"master"}`
   - **Notifications:** turn on "notify on failure"

   Use **Test run**. It should answer `204` and a run appears under Actions.

The token expires, at most a year after you create it. When it does, the cron-job.org
job fails with `401`: create a new token and paste it into the header.

History is kept in the Actions cache. If the cache is ever evicted, the next email goes
out without change figures and the history starts again.

## What the email says

Subject: `Testing 2. AUD/VND 18,238.1944 (-1.38%) and CZK/VND 1,230.1171 (+0.17%)`.
The number counts days of stored history, so it restarts if the history is lost.

```
AUD/VND   18,238.1900   -1.38%
  AUD leg  -1.32pp  AUD weaker vs USD
  VND leg  -0.06pp  VND stronger vs USD
         -1.15% vs your entry 18,450.0000

AUD/USD        0.6536   -1.32%
USD/VND        26,300   -0.06%
CZK/USD        0.0465   +0.31%
CZK/VND    1,223.4000   +0.25%
  CZK leg  +0.31pp  CZK stronger vs USD
  VND leg  -0.06pp  VND stronger vs USD
```

- **%** is a pair's own change since the previous publication.
- **pp** is how much of the AUD/VND or CZK/VND change each leg accounts for. The two legs
  always add up to that pair's figure.
- The VND leg is USD/VND, so a **positive** VND leg means the dong got **weaker**. The
  words next to each leg say which way it went; trust those over the sign.
- All rates are mid-market. What you receive on a transfer is lower by the provider's fee.

## Author

Original project: Loris Occhipinti ([Ipanov7/forex-bot](https://github.com/Ipanov7/forex-bot))
* ✉️Contact me at: loris@lorisocchipinti.com
* ⭐Website: https://blog.lorisocchipinti.com
