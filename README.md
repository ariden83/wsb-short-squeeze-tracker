# WSB Short Squeeze Tracker

Tracks stock tickers mentioned on [r/wallstreetbets](https://www.reddit.com/r/wallstreetbets/), ranked by most recent appearance. Each post's title is translated (EN→FR) and scored for sentiment (bullish / bearish / neutral) by a local NLP service — no external API calls.

## How it works

- A job runs every hour, but **only during NYSE trading hours** (Mon–Fri, 09:30–16:00 ET)
- It fetches new posts from r/wallstreetbets via the Reddit JSON API (no auth required)
- Tickers are extracted from titles and bodies:
  - `$TICKER` format (1–5 letters, case-insensitive)
  - Bare uppercase words (3–5 letters)
- Each candidate is validated against the **official SEC ticker list** (refreshed daily)
- Results are persisted in a local SQLite database
- A cursor (last seen post ID) prevents re-parsing already processed posts
- The web UI shows tickers ranked by last appearance, with mention counts over the last **30 days**
- When a ticker's detail page is opened, the post titles are translated (argostranslate, offline) and sentiment-analyzed (VADER) by a local Python microservice — results are cached in the DB

## Requirements

- Go 1.25+
- GCC (required by `go-sqlite3` for CGo)
- Python 3 with `venv` (for the NLP microservice — auto-installed by `make run`)

## Run

```bash
make run
```

This will:
1. Create a Python venv in `.venv/` and install NLP dependencies
2. Start the NLP service on `127.0.0.1:5001` in the background
3. Start the Go server on `:8086`

Then open [http://localhost:8086](http://localhost:8086).

Press `Ctrl-C` to stop (the NLP service is killed automatically).

> ℹ️ On first launch, the NLP service downloads the `en→fr` translation pack (~50 MB), cached locally for subsequent runs.

## Configuration

| Environment variable | Default | Description |
|---|---|---|
| `PORT` | `8086` | HTTP listening port |

## Endpoints

| Route | Method | Description |
|---|---|---|
| `/` | GET | Web UI (ticker list) |
| `/reddit.html` | GET | Web UI (ticker detail page) |
| `/api/tickers` | GET | Ticker stats as JSON (last 30 days) |
| `/api/tickers/{ticker}` | GET | Last 10 posts for a given ticker, with translation + sentiment |
| `/api/run` | POST | Trigger a manual job run |
| `/api/reset` | POST | Reset the cache (mentions + cursor) |

## Project structure

```
.
├── main.go             # HTTP server + hourly scheduler + market hours check
├── reddit.go           # Reddit JSON API client (incremental fetch)
├── ticker.go           # Ticker extraction (regex)
├── tickers_loader.go   # SEC ticker list loader + daily refresh
├── known_tickers.go    # Fallback embedded ticker list (S&P 500)
├── analyzer.go         # HTTP client to the local NLP microservice
├── store.go            # SQLite storage (posts, mentions, cursor, analysis)
├── nlp_service.py      # Python NLP microservice (translation + sentiment)
├── requirements_nlp.txt
├── static/
│   ├── index.html      # Ticker list page
│   └── reddit.html     # Ticker detail page
├── cmd/
│   └── debug/          # Debug utilities
├── Makefile
└── go.mod
```

## NLP microservice

A small Python HTTP server (`nlp_service.py`) listens on `127.0.0.1:5001` and exposes a single endpoint:

- `POST /analyze` — accepts a JSON array of titles, returns `{translation, sentiment}` for each

Stack:
- **Translation** — [argostranslate](https://github.com/argosopentech/argos-translate) (offline, EN→FR)
- **Sentiment** — [VADER](https://github.com/cjhutto/vaderSentiment) (social-media-tuned lexicon, well suited to WSB slang)

If the NLP service is unreachable, the Go server skips analysis silently — the rest of the app still works.

## Notes

- Reddit's public JSON API is used without authentication — no API key required
- A 2-second delay is added between paginated requests to respect Reddit's rate limits
- Ticker validation uses the [SEC company_tickers.json](https://www.sec.gov/files/company_tickers.json) file (refreshed every 24h); a fallback embedded list ships in `known_tickers.go` if the SEC fetch fails
- US market holidays are not handled; the job runs on all weekdays during market hours

## Disclaimer

This project is provided for **educational and informational purposes only**. It is not financial advice, investment advice, or a recommendation to buy or sell any security. The ticker mentions and sentiment scores are extracted from public Reddit posts and may be inaccurate, biased, or manipulated. Do your own research before making any financial decision.
