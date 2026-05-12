package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	redditBase = "https://www.reddit.com"
	userAgent  = "short-squeeze-tracker/1.0 (educational project)"
	pageSize   = 100
)

type redditListing struct {
	Data struct {
		After    string `json:"after"`
		Children []struct {
			Data struct {
				ID        string  `json:"id"`
				Title     string  `json:"title"`
				Selftext  string  `json:"selftext"`
				CreatedAt float64 `json:"created_utc"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type Post struct {
	ID        string
	Title     string
	Body      string
	CreatedAt time.Time
}

type RedditClient struct {
	http *http.Client
}

func NewRedditClient() *RedditClient {
	return &RedditClient{
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchNewPosts fetches new posts from r/wallstreetbets,
// stopping when it encounters stopAtID (the last cached post ID).
// Returns posts in reverse-chronological order (newest first).
func (c *RedditClient) FetchNewPosts(stopAtID string) ([]Post, string, error) {
	var posts []Post
	var newestID string
	after := ""

	for {
		page, err := c.fetchPage(after)
		if err != nil {
			return posts, newestID, err
		}
		if len(page.Data.Children) == 0 {
			break
		}

		done := false
		for _, child := range page.Data.Children {
			d := child.Data
			if d.ID == stopAtID {
				done = true
				break
			}
			if newestID == "" {
				newestID = d.ID
			}
			posts = append(posts, Post{
				ID:        d.ID,
				Title:     d.Title,
				Body:      d.Selftext,
				CreatedAt: time.Unix(int64(d.CreatedAt), 0),
			})
		}

		if done || page.Data.After == "" {
			break
		}
		after = page.Data.After

		// Be polite with Reddit's rate limits
		time.Sleep(2 * time.Second)
	}

	return posts, newestID, nil
}

func (c *RedditClient) fetchPage(after string) (*redditListing, error) {
	params := url.Values{}
	params.Set("limit", fmt.Sprintf("%d", pageSize))
	if after != "" {
		params.Set("after", after)
	}

	reqURL := fmt.Sprintf("%s/r/wallstreetbets/new.json?%s", redditBase, params.Encode())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit returned status %d", resp.StatusCode)
	}

	var listing redditListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	return &listing, nil
}
