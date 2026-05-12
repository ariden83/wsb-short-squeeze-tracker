package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	dbPath     = "reddits.db"
	httpAddr   = ":8086"
	statWindow = 30 * 24 * time.Hour // show mentions from last 30 days
)

// isMarketOpen returns true if t falls within NYSE trading hours:
// Monday–Friday, 09:30–16:00 ET.
func isMarketOpen(t time.Time) bool {
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	et := t.In(nyc)
	wd := et.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	h, m, _ := et.Clock()
	mins := h*60 + m
	return mins >= 9*60+30 && mins < 16*60
}

func fetchAndSave(store *Store, client *RedditClient) {
	log.Println("job started")

	cursor, err := store.GetCursor()
	if err != nil {
		log.Printf("get cursor error: %v", err)
		return
	}

	posts, newestID, err := client.FetchNewPosts(cursor)
	if err != nil {
		log.Printf("fetch error: %v", err)
		return
	}
	log.Printf("fetched %d new posts", len(posts))

	var postsWithTickers, totalMentions int
	for _, p := range posts {
		tickers := ExtractTickers(p.Title, p.Body)
		if len(tickers) == 0 {
			continue
		}
		if err := store.SavePost(p.ID, p.Title, p.Body, p.CreatedAt); err != nil {
			log.Printf("save post error (post %s): %v", p.ID, err)
		}
		postsWithTickers++
		totalMentions += len(tickers)
		if err := store.SaveMentions(tickers, p.ID, p.CreatedAt); err != nil {
			log.Printf("save mentions error (post %s): %v", p.ID, err)
		}
	}
	log.Printf("posts with tickers: %d, total mention records: %d", postsWithTickers, totalMentions)

	if newestID != "" {
		if err := store.SetCursor(newestID); err != nil {
			log.Printf("set cursor error: %v", err)
		}
	}
	log.Println("job done")
}

func main() {
	store, err := NewStore(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	client := NewRedditClient()

	// Load SEC tickers immediately, refresh daily
	scheduleSECRefresh()

	// Always collect on startup to fill the cache if empty
	go fetchAndSave(store, client)

	// Hourly job: only runs during NYSE market hours
	go func() {
		for range time.Tick(time.Hour) {
			if isMarketOpen(time.Now()) {
				go fetchAndSave(store, client)
			} else {
				log.Println("market closed, skipping hourly job")
			}
		}
	}()

	// HTTP handlers
	mux := http.NewServeMux()

	// Serve static frontend
	mux.Handle("/", http.FileServer(http.Dir("static")))

	// API endpoint: return ticker stats as JSON
	mux.HandleFunc("/api/tickers", func(w http.ResponseWriter, r *http.Request) {
		stats, err := store.GetStats(statWindow)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Printf("GetStats error: %v", err)
			return
		}
		log.Printf("GetStats returned %d tickers", len(stats))
		type response struct {
			Ticker   string `json:"ticker"`
			Count    int    `json:"count"`
			LastSeen string `json:"last_seen"`
		}
		out := make([]response, 0, len(stats))
		for _, s := range stats {
			out = append(out, response{
				Ticker:   s.Ticker,
				Count:    s.Count,
				LastSeen: s.LastSeen.UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	// API endpoint: posts for a specific ticker
	mux.HandleFunc("/api/tickers/{ticker}", func(w http.ResponseWriter, r *http.Request) {
		ticker := r.PathValue("ticker")
		posts, err := store.GetPostsForTicker(ticker, 10)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Printf("GetPostsForTicker error: %v", err)
			return
		}
		// Analyze posts that haven't been processed yet
		var toAnalyze []int
		var titles []string
		for i, p := range posts {
			if p.Translation == "" {
				toAnalyze = append(toAnalyze, i)
				titles = append(titles, p.Title)
			}
		}
		if len(titles) > 0 {
			analyses, err := AnalyzePosts(titles)
			if err != nil {
				log.Printf("analyze posts error: %v", err)
			} else if analyses != nil {
				for j, idx := range toAnalyze {
					posts[idx].Translation = analyses[j].Translation
					posts[idx].Sentiment = analyses[j].Sentiment
					if err := store.UpdatePostAnalysis(posts[idx].ID, analyses[j].Translation, analyses[j].Sentiment); err != nil {
						log.Printf("update analysis error: %v", err)
					}
				}
			}
		}

		type postResponse struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Translation string `json:"translation"`
			Sentiment   string `json:"sentiment"`
			URL         string `json:"url"`
			SeenAt      string `json:"seen_at"`
			SqueezeHit  bool   `json:"squeeze_hit"`
		}
		out := make([]postResponse, 0, len(posts))
		for _, p := range posts {
			out = append(out, postResponse{
				ID:          p.ID,
				Title:       p.Title,
				Translation: p.Translation,
				Sentiment:   p.Sentiment,
				URL:         "https://www.reddit.com/r/wallstreetbets/comments/" + p.ID,
				SeenAt:      p.SeenAt.UTC().Format(time.RFC3339),
				SqueezeHit:  p.SqueezeHit,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	// API endpoint: trigger manual job run
	mux.HandleFunc("/api/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go fetchAndSave(store, client)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"triggered"}`))
	})

	// API endpoint: reset cache (mentions + cursor)
	mux.HandleFunc("/api/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := store.Reset(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Printf("Reset error: %v", err)
			return
		}
		log.Println("cache reset")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"reset"}`))
	})


	port := os.Getenv("PORT")
	if port != "" {
		log.Printf("listening on :%s", port)
		log.Fatal(http.ListenAndServe(":"+port, mux))
	} else {
		log.Printf("listening on %s", httpAddr)
		log.Fatal(http.ListenAndServe(httpAddr, mux))
	}
}
