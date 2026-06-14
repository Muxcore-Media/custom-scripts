package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	metadatav1 "github.com/Muxcore-Media/metadata-tmdb/proto/metadatav1"
)

func newTestServer(t *testing.T) (*httptest.Server, *Module) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/configuration":
			json.NewEncoder(w).Encode(map[string]any{
				"images": map[string]any{
					"base_url":        "http://image.tmdb.org/t/p/",
					"secure_base_url": "https://image.tmdb.org/t/p/",
					"poster_sizes":    []string{"w92", "w154", "w185", "w342", "w500", "w780", "original"},
					"backdrop_sizes":  []string{"w300", "w780", "w1280", "original"},
					"logo_sizes":      []string{"w45", "w92", "w154", "w185", "w300", "w500", "original"},
					"profile_sizes":   []string{"w45", "w185", "h632", "original"},
					"still_sizes":     []string{"w92", "w185", "w300", "original"},
				},
			})
		case "/3/search/movie":
			json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "total_results": 1, "total_pages": 1,
				"results": []map[string]any{
					{
						"id": 550, "title": "Fight Club",
						"overview":     "A ticking-clock thriller.",
						"poster_path":  "/pB8BM7pdSp6B6Ih7QZ4DrQ3PmJK.jpg",
						"release_date": "1999-10-15",
						"vote_average": 8.4, "vote_count": 25000,
						"media_type": "movie",
						"genre_ids":  []int32{18, 53},
					},
				},
			})
		case "/3/search/tv":
			json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "total_results": 1, "total_pages": 1,
				"results": []map[string]any{
					{
						"id": 1396, "name": "Breaking Bad",
						"overview":       "A high school chemistry teacher.",
						"poster_path":    "/ggFHVNu6YYI5L9W6QN5CvfWgmd.jpg",
						"first_air_date": "2008-01-20",
						"vote_average":   8.9, "vote_count": 10000,
						"media_type": "tv",
					},
				},
			})
		case "/3/movie/550":
			json.NewEncoder(w).Encode(map[string]any{
				"id": 550, "title": "Fight Club",
				"original_title": "Fight Club",
				"overview":       "A ticking-clock thriller.",
				"tagline":        "Mischief. Mayhem. Soap.",
				"poster_path":    "/pB8BM7pdSp6B6Ih7QZ4DrQ3PmJK.jpg",
				"release_date":   "1999-10-15",
				"runtime":        139,
				"vote_average":   8.4, "vote_count": 25000,
				"status":               "Released",
				"budget":               63000000,
				"revenue":              100853753,
				"imdb_id":              "tt0137523",
				"genres":               []map[string]any{{"id": 18, "name": "Drama"}, {"id": 53, "name": "Thriller"}},
				"production_companies": []map[string]any{{"id": 508, "name": "20th Century Fox", "origin_country": "US"}},
				"spoken_languages":     []map[string]any{{"iso_639_1": "en", "name": "English"}},
			})
		case "/3/tv/1396":
			json.NewEncoder(w).Encode(map[string]any{
				"id": 1396, "name": "Breaking Bad",
				"original_name":      "Breaking Bad",
				"overview":           "A high school chemistry teacher.",
				"first_air_date":     "2008-01-20",
				"last_air_date":      "2013-09-29",
				"number_of_seasons":  5,
				"number_of_episodes": 62,
				"vote_average":       8.9, "vote_count": 10000,
				"status": "Ended",
				"genres": []map[string]any{{"id": 18, "name": "Drama"}, {"id": 80, "name": "Crime"}},
				"seasons": []map[string]any{
					{"id": 1, "name": "Season 1", "season_number": 1, "episode_count": 7, "air_date": "2008-01-20"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"status_message": "Not found"})
		}
	}))

	m := NewModule(Config{
		GRPCAddr: ":0",
		APIKey:   "test-api-key",
		BaseURL:  srv.URL,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { m.Stop(ctx) })

	return srv, m
}

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if len(info.Roles) == 0 || info.Roles[0] != "metadata" {
		t.Errorf("expected role metadata, got %v", info.Roles)
	}
	if len(info.Capabilities) == 0 || info.Capabilities[0] != "metadata" {
		t.Errorf("expected capability metadata, got %v", info.Capabilities)
	}
}

func TestSearchMovie(t *testing.T) {
	srv, m := newTestServer(t)
	defer srv.Close()
	ctx := context.Background()

	resp, err := m.Search(ctx, &metadatav1.SearchRequest{
		Query: "fight club",
		Type:  metadatav1.MediaType_MEDIA_TYPE_MOVIE,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Fight Club" {
		t.Errorf("expected 'Fight Club', got %s", resp.Results[0].Title)
	}
	if resp.Results[0].Id != 550 {
		t.Errorf("expected id 550, got %d", resp.Results[0].Id)
	}
}

func TestSearchTV(t *testing.T) {
	srv, m := newTestServer(t)
	defer srv.Close()
	ctx := context.Background()

	resp, err := m.Search(ctx, &metadatav1.SearchRequest{
		Query: "breaking bad",
		Type:  metadatav1.MediaType_MEDIA_TYPE_TV,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Name != "Breaking Bad" {
		t.Errorf("expected 'Breaking Bad', got %s", resp.Results[0].Name)
	}
}

func TestGetMovieDetails(t *testing.T) {
	srv, m := newTestServer(t)
	defer srv.Close()
	ctx := context.Background()

	if _, err := m.GetConfiguration(ctx, &metadatav1.GetConfigurationRequest{}); err != nil {
		t.Fatal(err)
	}

	resp, err := m.GetMovieDetails(ctx, &metadatav1.GetMovieDetailsRequest{TmdbId: 550})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Fight Club" {
		t.Errorf("expected 'Fight Club', got %s", resp.Title)
	}
	if resp.Runtime != 139 {
		t.Errorf("expected runtime 139, got %d", resp.Runtime)
	}
	if resp.ImdbId != "tt0137523" {
		t.Errorf("expected imdb tt0137523, got %s", resp.ImdbId)
	}
	if len(resp.Genres) != 2 {
		t.Fatalf("expected 2 genres, got %d", len(resp.Genres))
	}
	if resp.Genres[0].Name != "Drama" {
		t.Errorf("expected genre Drama, got %s", resp.Genres[0].Name)
	}
	if resp.PosterUrl == "" {
		t.Error("expected poster URL")
	}
}

func TestGetTVDetails(t *testing.T) {
	srv, m := newTestServer(t)
	defer srv.Close()
	ctx := context.Background()

	if _, err := m.GetConfiguration(ctx, &metadatav1.GetConfigurationRequest{}); err != nil {
		t.Fatal(err)
	}

	resp, err := m.GetTVDetails(ctx, &metadatav1.GetTVDetailsRequest{TmdbId: 1396})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != "Breaking Bad" {
		t.Errorf("expected 'Breaking Bad', got %s", resp.Name)
	}
	if resp.NumberOfSeasons != 5 {
		t.Errorf("expected 5 seasons, got %d", resp.NumberOfSeasons)
	}
	if len(resp.Seasons) != 1 {
		t.Fatalf("expected 1 season, got %d", len(resp.Seasons))
	}
	if resp.Seasons[0].Name != "Season 1" {
		t.Errorf("expected 'Season 1', got %s", resp.Seasons[0].Name)
	}
}

func TestGetConfiguration(t *testing.T) {
	srv, m := newTestServer(t)
	defer srv.Close()
	ctx := context.Background()

	resp, err := m.GetConfiguration(ctx, &metadatav1.GetConfigurationRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.BaseUrl == "" {
		t.Error("expected base URL")
	}
	if len(resp.PosterSizes) == 0 {
		t.Error("expected poster sizes")
	}
}

func TestHealthNoAPIKey(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()
	if err := m.Health(ctx); err == nil {
		t.Error("expected health error without API key")
	}
}

func TestHealthWithAPIKey(t *testing.T) {
	m := NewModule(Config{APIKey: "test-key"})
	ctx := context.Background()
	if err := m.Health(ctx); err != nil {
		t.Errorf("expected health ok with API key, got %v", err)
	}
}

func TestLifecycle(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
