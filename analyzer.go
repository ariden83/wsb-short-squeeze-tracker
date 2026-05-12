package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const nlpServiceURL = "http://127.0.0.1:5001/analyze"

type PostAnalysis struct {
	Translation string
	Sentiment   string // "bullish", "bearish", "neutral"
}

// AnalyzePosts sends titles to the local Python NLP service.
// Returns nil (no error) if the service is unreachable — analysis is optional.
func AnalyzePosts(titles []string) ([]PostAnalysis, error) {
	if len(titles) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(titles)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(nlpServiceURL, "application/json", bytes.NewReader(body))
	if err != nil {
		// Service not running — skip analysis silently
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nlp service returned status %d", resp.StatusCode)
	}

	var results []PostAnalysis
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	if len(results) != len(titles) {
		return nil, fmt.Errorf("expected %d results, got %d", len(titles), len(results))
	}
	return results, nil
}
