package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const secTickersURL = "https://www.sec.gov/files/company_tickers.json"

var (
	tickersMu     sync.RWMutex
	activeTickers = knownTickers // fallback to static list until SEC loads
)

func isKnownTicker(t string) bool {
	tickersMu.RLock()
	defer tickersMu.RUnlock()
	return activeTickers[t]
}

// loadSECTickers fetches the full list of US-listed tickers from SEC EDGAR
// and replaces the active ticker set. Falls back to the static list on error.
func loadSECTickers() error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", secTickersURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("SEC request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SEC returned status %d", resp.StatusCode)
	}

	var raw map[string]struct {
		Ticker string `json:"ticker"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	tickers := make(map[string]bool, len(raw))
	for _, entry := range raw {
		if t := strings.ToUpper(entry.Ticker); t != "" {
			tickers[t] = true
		}
	}

	tickersMu.Lock()
	activeTickers = tickers
	tickersMu.Unlock()

	log.Printf("loaded %d tickers from SEC EDGAR", len(tickers))
	return nil
}

// scheduleSECRefresh loads tickers immediately then refreshes every 24h.
func scheduleSECRefresh() {
	if err := loadSECTickers(); err != nil {
		log.Printf("SEC tickers load failed, using static fallback: %v", err)
	}
	go func() {
		for range time.Tick(24 * time.Hour) {
			if err := loadSECTickers(); err != nil {
				log.Printf("SEC tickers refresh failed: %v", err)
			}
		}
	}()
}
