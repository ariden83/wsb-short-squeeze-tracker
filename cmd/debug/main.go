package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const userAgent = "short-squeeze-tracker/1.0 (educational project)"

func main() {
	fmt.Println("=== DB check ===")
	db, err := sql.Open("sqlite3", "reddits.db")
	if err != nil {
		fmt.Println("DB open error:", err)
	} else {
		defer db.Close()
		var count int
		db.QueryRow("SELECT COUNT(*) FROM mentions").Scan(&count)
		fmt.Println("Total mention rows:", count)

		var cursor string
		db.QueryRow("SELECT last_post FROM cursor WHERE id=1").Scan(&cursor)
		fmt.Println("Cursor (last post ID):", cursor)

		rows, _ := db.Query(`
			SELECT ticker, COUNT(*) as c, MAX(seen_at) as last
			FROM mentions GROUP BY ticker ORDER BY c DESC LIMIT 10
		`)
		if rows != nil {
			defer rows.Close()
			fmt.Println("Top 10 tickers:")
			for rows.Next() {
				var t string
				var c int
				var last int64
				rows.Scan(&t, &c, &last)
				fmt.Printf("  %-10s %d mentions  last=%s\n", t, c, time.Unix(last, 0).Format(time.RFC3339))
			}
		}

		cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
		var recent int
		db.QueryRow("SELECT COUNT(*) FROM mentions WHERE seen_at >= ?", cutoff).Scan(&recent)
		fmt.Printf("Mentions in last 7 days (seen_at >= %d): %d\n", cutoff, recent)
	}

	fmt.Println("\n=== Reddit API call (5 posts) ===")
	params := url.Values{}
	params.Set("q", "short squeeze")
	params.Set("sort", "new")
	params.Set("limit", "5")
	params.Set("restrict_sr", "true")
	reqURL := "https://www.reddit.com/r/wallstreetbets/search.json?" + params.Encode()

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	fmt.Println("HTTP status:", resp.StatusCode)

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		fmt.Println("JSON decode error:", err)
		os.Exit(1)
	}

	data, _ := raw["data"].(map[string]any)
	children, _ := data["children"].([]any)
	fmt.Printf("got %d posts\n\n", len(children))

	fmt.Println("=== Ticker extraction (first 3 posts) ===")
	for i, child := range children {
		if i >= 3 {
			break
		}
		c, _ := child.(map[string]any)
		d, _ := c["data"].(map[string]any)
		id, _ := d["id"].(string)
		title, _ := d["title"].(string)
		body, _ := d["selftext"].(string)
		created, _ := d["created_utc"].(float64)

		fmt.Printf("[%d] id=%s  created=%s\n", i+1, id, time.Unix(int64(created), 0).Format(time.RFC3339))
		fmt.Printf("    title:   %s\n", title)
		tickers := extractTickers(title + " " + body)
		fmt.Printf("    tickers: %v\n\n", tickers[:min(len(tickers), 5)])
	}
}

var blocklist = map[string]bool{
	"I": true, "A": true, "AM": true, "PM": true, "US": true, "UK": true,
	"EU": true, "FBI": true, "SEC": true, "FDA": true, "IRS": true,
	"IPO": true, "CEO": true, "CFO": true, "COO": true, "CTO": true,
	"ETF": true, "WSB": true, "DD": true, "OP": true, "OG": true,
	"IV": true, "EV": true, "AI": true, "IT": true, "IF": true,
	"OR": true, "AND": true, "THE": true, "FOR": true, "NOT": true,
	"ALL": true, "ARE": true, "BUT": true, "CAN": true, "DID": true,
	"GET": true, "GOT": true, "HAS": true, "HAD": true, "HIT": true,
	"HOW": true, "ITS": true, "LET": true, "LOL": true, "NEW": true,
	"NOW": true, "OLD": true, "ONE": true, "OUR": true, "OUT": true,
	"OWN": true, "RUN": true, "SAW": true, "SAY": true, "SEE": true,
	"SET": true, "SO": true, "TO": true, "TOO": true, "TOP": true,
	"TWO": true, "USE": true, "VIA": true, "WAY": true, "WAS": true,
	"WE": true, "WHO": true, "WHY": true, "WIN": true, "WON": true,
	"YOY": true, "YTD": true, "ATH": true, "ATL": true, "AMA": true,
	"EDIT": true, "TLDR": true, "YOLO": true, "FOMO": true, "FUD": true,
	"MOON": true, "BEAR": true, "BULL": true, "CALL": true, "PUT": true,
	"PUTS": true, "CALLS": true, "BUY": true, "SELL": true, "HOLD": true,
	"USA": true, "NYSE": true, "NASDAQ": true, "OTC": true,
}

var dollarRe = regexp.MustCompile(`\$([A-Z]{1,5})\b`)
var capsRe = regexp.MustCompile(`\b([A-Z]{2,5})\b`)

func extractTickers(text string) []string {
	upper := strings.ToUpper(text)
	seen := map[string]bool{}
	var result []string
	for _, m := range dollarRe.FindAllStringSubmatch(upper, -1) {
		t := m[1]
		if !blocklist[t] && !seen[t] {
			seen[t] = true
			result = append(result, "$"+t)
		}
	}
	for _, m := range capsRe.FindAllStringSubmatch(upper, -1) {
		t := m[1]
		if !blocklist[t] && !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
