package main

import (
	"regexp"
	"strings"
)

// dollarTicker matches $GME or $gme style (case-insensitive prefix), 1-5 letters.
var dollarTicker = regexp.MustCompile(`(?i)\$([A-Za-z]{1,5})\b`)

// capsWord matches standalone uppercase words of 3-5 letters in the original text.
// 1-2 letter tickers are too ambiguous as bare words; they require the $ prefix.
var capsWord = regexp.MustCompile(`\b([A-Z]{3,5})\b`)

// ExtractTickers extracts stock tickers from a post title and body.
// Strategy:
//   - $TICKER format (1-5 letters): case-insensitive, accepted if in knownTickers
//   - Bare caps words (3-5 letters): only matches words already written in uppercase,
//     avoiding false positives from common lowercase words and single/double letters.
func ExtractTickers(title, body string) []string {
	text := title + " " + body
	seen := map[string]bool{}
	var result []string

	add := func(t string) {
		t = strings.ToUpper(t)
		if isKnownTicker(t) && !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}

	// $TICKER: case-insensitive, all lengths (e.g. $T, $gme, $AAPL)
	for _, m := range dollarTicker.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	// Bare caps: 3+ letters only, already uppercase in original text
	for _, m := range capsWord.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}

	return result
}
