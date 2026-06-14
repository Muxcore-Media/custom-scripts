package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	metadatav1 "github.com/Muxcore-Media/metadata-tmdb/proto/metadatav1"
)

type tmdbConfig struct {
	Images struct {
		BaseURL       string   `json:"base_url"`
		SecureBaseURL string   `json:"secure_base_url"`
		PosterSizes   []string `json:"poster_sizes"`
		BackdropSizes []string `json:"backdrop_sizes"`
		LogoSizes     []string `json:"logo_sizes"`
		ProfileSizes  []string `json:"profile_sizes"`
		StillSizes    []string `json:"still_sizes"`
	} `json:"images"`
}

type Module struct {
	metadatav1.UnimplementedMetadataServiceServer

	mu         sync.RWMutex
	client     *http.Client
	apiKey     string
	baseURL    string
	tmdbConfig *tmdbConfig
	configTime time.Time
	configTTL  time.Duration

	id       string
	grpcAddr string
	grpcSrv  *grpc.Server
	lis      net.Listener
}

type Config struct {
	ID       string
	GRPCAddr string
	APIKey   string
	Timeout  time.Duration
	BaseURL  string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "metadata-tmdb"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9410"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.themoviedb.org"
	}
	if v := os.Getenv("MUXCORE_CFG_TMDB_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("TMDB_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("METADATA_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("TMDB_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	return &Module{
		id:        cfg.ID,
		grpcAddr:  cfg.GRPCAddr,
		apiKey:    cfg.APIKey,
		baseURL:   cfg.BaseURL,
		configTTL: 24 * time.Hour,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Metadata TMDB",
		Version:      "0.1.0",
		Roles:        []string{"metadata"},
		Description:  "TMDB (The Movie Database) metadata provider for movies and TV shows",
		Author:       "MuxCore",
		Capabilities: []string{"metadata"},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/contracts-metadata",
				Interface: "MetadataProvider",
				Version:   "v0.1.0",
			},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	slog.Info("metadata-tmdb initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	metadatav1.RegisterMetadataServiceServer(m.grpcSrv, m)
	go func() {
		slog.Info("metadata-tmdb gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("metadata-tmdb gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	m.client.CloseIdleConnections()
	slog.Info("metadata-tmdb stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.apiKey == "" {
		return fmt.Errorf("TMDB API key not configured — set TMDB_API_KEY or MUXCORE_CFG_TMDB_API_KEY")
	}
	return nil
}

func (m *Module) Search(ctx context.Context, req *metadatav1.SearchRequest) (*metadatav1.SearchResponse, error) {
	params := url.Values{}
	params.Set("query", req.GetQuery())
	params.Set("page", strconv.Itoa(int(req.GetPage())))
	if req.GetPage() <= 0 {
		params.Set("page", "1")
	}
	if req.GetLanguage() != "" {
		params.Set("language", req.GetLanguage())
	}

	var endpoint string
	switch req.GetType() {
	case metadatav1.MediaType_MEDIA_TYPE_MOVIE:
		endpoint = "/3/search/movie"
	case metadatav1.MediaType_MEDIA_TYPE_TV:
		endpoint = "/3/search/tv"
	default:
		endpoint = "/3/search/multi"
	}

	var raw struct {
		Results      []json.RawMessage `json:"results"`
		TotalResults int               `json:"total_results"`
		TotalPages   int               `json:"total_pages"`
		Page         int               `json:"page"`
	}
	if err := m.tmdbGet(ctx, endpoint, params, &raw); err != nil {
		return nil, err
	}

	results := make([]*metadatav1.SearchResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		sr := m.parseSearchResult(r)
		if sr != nil {
			results = append(results, sr)
		}
	}

	return &metadatav1.SearchResponse{
		Results:      results,
		TotalResults: int32(raw.TotalResults),
		TotalPages:   int32(raw.TotalPages),
		Page:         int32(raw.Page),
	}, nil
}

func (m *Module) GetMovieDetails(ctx context.Context, req *metadatav1.GetMovieDetailsRequest) (*metadatav1.GetMovieDetailsResponse, error) {
	id := req.GetTmdbId()
	endpoint := fmt.Sprintf("/3/movie/%d", id)

	params := url.Values{}
	if req.GetLanguage() != "" {
		params.Set("language", req.GetLanguage())
	}
	if len(req.GetAppendToResponse()) > 0 {
		params.Set("append_to_response", strings.Join(req.GetAppendToResponse(), ","))
	}

	var raw movieDetailRaw
	if err := m.tmdbGet(ctx, endpoint, params, &raw); err != nil {
		return nil, err
	}

	return raw.toProto(m.getCachedConfig()), nil
}

func (m *Module) GetTVDetails(ctx context.Context, req *metadatav1.GetTVDetailsRequest) (*metadatav1.GetTVDetailsResponse, error) {
	id := req.GetTmdbId()
	endpoint := fmt.Sprintf("/3/tv/%d", id)

	params := url.Values{}
	if req.GetLanguage() != "" {
		params.Set("language", req.GetLanguage())
	}
	if len(req.GetAppendToResponse()) > 0 {
		params.Set("append_to_response", strings.Join(req.GetAppendToResponse(), ","))
	}

	var raw tvDetailRaw
	if err := m.tmdbGet(ctx, endpoint, params, &raw); err != nil {
		return nil, err
	}

	return raw.toProto(m.getCachedConfig()), nil
}

func (m *Module) GetConfiguration(ctx context.Context, req *metadatav1.GetConfigurationRequest) (*metadatav1.GetConfigurationResponse, error) {
	cfg, err := m.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &metadatav1.GetConfigurationResponse{
		BaseUrl:       cfg.Images.BaseURL,
		SecureBaseUrl: cfg.Images.SecureBaseURL,
		PosterSizes:   cfg.Images.PosterSizes,
		BackdropSizes: cfg.Images.BackdropSizes,
		LogoSizes:     cfg.Images.LogoSizes,
		ProfileSizes:  cfg.Images.ProfileSizes,
		StillSizes:    cfg.Images.StillSizes,
	}, nil
}

func (m *Module) tmdbGet(ctx context.Context, endpoint string, params url.Values, dest any) error {
	if m.apiKey == "" {
		return fmt.Errorf("TMDB API key not configured")
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", m.apiKey)

	u := m.baseURL + endpoint + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb returned %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode tmdb response: %w", err)
	}
	return nil
}

func (m *Module) getConfig(ctx context.Context) (*tmdbConfig, error) {
	m.mu.RLock()
	if m.tmdbConfig != nil && time.Since(m.configTime) < m.configTTL {
		cfg := m.tmdbConfig
		m.mu.RUnlock()
		return cfg, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tmdbConfig != nil && time.Since(m.configTime) < m.configTTL {
		return m.tmdbConfig, nil
	}

	var cfg tmdbConfig
	if err := m.tmdbGet(ctx, "/3/configuration", nil, &cfg); err != nil {
		return nil, fmt.Errorf("fetch tmdb config: %w", err)
	}

	m.tmdbConfig = &cfg
	m.configTime = time.Now()
	return &cfg, nil
}

func (m *Module) getCachedConfig() *tmdbConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tmdbConfig
}

func (m *Module) imageURL(path string, cfg *tmdbConfig) string {
	if path == "" || cfg == nil {
		return ""
	}
	return cfg.Images.SecureBaseURL + "original" + path
}

func (m *Module) parseSearchResult(raw json.RawMessage) *metadatav1.SearchResult {
	var base struct {
		ID               int     `json:"id"`
		Overview         string  `json:"overview"`
		PosterPath       string  `json:"poster_path"`
		BackdropPath     string  `json:"backdrop_path"`
		VoteAverage      float64 `json:"vote_average"`
		VoteCount        int     `json:"vote_count"`
		Popularity       float64 `json:"popularity"`
		OriginalLanguage string  `json:"original_language"`
		GenreIDs         []int32 `json:"genre_ids"`
		MediaType        string  `json:"media_type"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil
	}

	var movie struct {
		Title         string `json:"title"`
		OriginalTitle string `json:"original_title"`
		ReleaseDate   string `json:"release_date"`
	}
	var tv struct {
		Name         string `json:"name"`
		OriginalName string `json:"original_name"`
		FirstAirDate string `json:"first_air_date"`
	}
	json.Unmarshal(raw, &movie)
	json.Unmarshal(raw, &tv)

	result := &metadatav1.SearchResult{
		Id:               int32(base.ID),
		Overview:         base.Overview,
		PosterPath:       base.PosterPath,
		BackdropPath:     base.BackdropPath,
		VoteAverage:      base.VoteAverage,
		VoteCount:        int32(base.VoteCount),
		Popularity:       base.Popularity,
		OriginalLanguage: base.OriginalLanguage,
		GenreIds:         base.GenreIDs,
		Title:            movie.Title,
		OriginalTitle:    movie.OriginalTitle,
		ReleaseDate:      movie.ReleaseDate,
		Name:             tv.Name,
		FirstAirDate:     tv.FirstAirDate,
	}

	switch base.MediaType {
	case "movie":
		result.MediaType = metadatav1.MediaType_MEDIA_TYPE_MOVIE
	case "tv":
		result.MediaType = metadatav1.MediaType_MEDIA_TYPE_TV
	}

	return result
}

type movieDetailRaw struct {
	Adult            bool           `json:"adult"`
	Budget           int64          `json:"budget"`
	Homepage         string         `json:"homepage"`
	ID               int            `json:"id"`
	IMDBID           string         `json:"imdb_id"`
	OriginalLanguage string         `json:"original_language"`
	OriginalTitle    string         `json:"original_title"`
	Overview         string         `json:"overview"`
	Popularity       float64        `json:"popularity"`
	PosterPath       string         `json:"poster_path"`
	BackdropPath     string         `json:"backdrop_path"`
	ReleaseDate      string         `json:"release_date"`
	Revenue          int64          `json:"revenue"`
	Runtime          int            `json:"runtime"`
	Status           string         `json:"status"`
	Tagline          string         `json:"tagline"`
	Title            string         `json:"title"`
	VoteAverage      float64        `json:"vote_average"`
	VoteCount        int            `json:"vote_count"`
	Genres           []genreRaw     `json:"genres"`
	ProductionCos    []companyRaw   `json:"production_companies"`
	SpokenLangs      []languageRaw  `json:"spoken_languages"`
	BelongsTo        *collectionRaw `json:"belongs_to_collection"`
}

type genreRaw struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type companyRaw struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoPath      string `json:"logo_path"`
	OriginCountry string `json:"origin_country"`
}

type languageRaw struct {
	ISO6391 string `json:"iso_639_1"`
	Name    string `json:"name"`
}

type collectionRaw struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PosterPath   string `json:"poster_path"`
	BackdropPath string `json:"backdrop_path"`
}

func (r *movieDetailRaw) toProto(cfg *tmdbConfig) *metadatav1.GetMovieDetailsResponse {
	resp := &metadatav1.GetMovieDetailsResponse{
		Id:               int32(r.ID),
		Title:            r.Title,
		OriginalTitle:    r.OriginalTitle,
		Overview:         r.Overview,
		Tagline:          r.Tagline,
		PosterPath:       r.PosterPath,
		BackdropPath:     r.BackdropPath,
		ReleaseDate:      r.ReleaseDate,
		Runtime:          int32(r.Runtime),
		VoteAverage:      r.VoteAverage,
		VoteCount:        int32(r.VoteCount),
		Popularity:       r.Popularity,
		Status:           r.Status,
		OriginalLanguage: r.OriginalLanguage,
		Adult:            r.Adult,
		Budget:           r.Budget,
		Revenue:          r.Revenue,
		Homepage:         r.Homepage,
		ImdbId:           r.IMDBID,
	}
	if cfg != nil {
		resp.PosterUrl = cfg.Images.SecureBaseURL + "original" + r.PosterPath
		resp.BackdropUrl = cfg.Images.SecureBaseURL + "original" + r.BackdropPath
	}
	for _, g := range r.Genres {
		resp.Genres = append(resp.Genres, &metadatav1.Genre{Id: int32(g.ID), Name: g.Name})
	}
	for _, c := range r.ProductionCos {
		resp.ProductionCompanies = append(resp.ProductionCompanies, &metadatav1.ProductionCompany{
			Id: int32(c.ID), Name: c.Name, LogoPath: c.LogoPath, OriginCountry: c.OriginCountry,
		})
	}
	for _, l := range r.SpokenLangs {
		resp.SpokenLanguages = append(resp.SpokenLanguages, &metadatav1.SpokenLanguage{Iso_639_1: l.ISO6391, Name: l.Name})
	}
	if r.BelongsTo != nil {
		resp.BelongsToCollection = &metadatav1.Collection{
			Id: int32(r.BelongsTo.ID), Name: r.BelongsTo.Name,
			PosterPath: r.BelongsTo.PosterPath, BackdropPath: r.BelongsTo.BackdropPath,
		}
	}
	return resp
}

type tvDetailRaw struct {
	ID               int           `json:"id"`
	Name             string        `json:"name"`
	OriginalName     string        `json:"original_name"`
	Overview         string        `json:"overview"`
	Tagline          string        `json:"tagline"`
	PosterPath       string        `json:"poster_path"`
	BackdropPath     string        `json:"backdrop_path"`
	FirstAirDate     string        `json:"first_air_date"`
	LastAirDate      string        `json:"last_air_date"`
	NumSeasons       int           `json:"number_of_seasons"`
	NumEpisodes      int           `json:"number_of_episodes"`
	VoteAverage      float64       `json:"vote_average"`
	VoteCount        int           `json:"vote_count"`
	Popularity       float64       `json:"popularity"`
	Status           string        `json:"status"`
	OriginalLanguage string        `json:"original_language"`
	Homepage         string        `json:"homepage"`
	InProduction     bool          `json:"in_production"`
	Genres           []genreRaw    `json:"genres"`
	Seasons          []seasonRaw   `json:"seasons"`
	ProductionCos    []companyRaw  `json:"production_companies"`
	SpokenLangs      []languageRaw `json:"spoken_languages"`
	CreatedBy        []creatorRaw  `json:"created_by"`
	Networks         []networkRaw  `json:"networks"`
	OriginCountries  []string      `json:"origin_country"`
}

type seasonRaw struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	AirDate      string  `json:"air_date"`
	SeasonNumber int     `json:"season_number"`
	EpisodeCount int     `json:"episode_count"`
	VoteAverage  float64 `json:"vote_average"`
}

type creatorRaw struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
}

type networkRaw struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoPath      string `json:"logo_path"`
	OriginCountry string `json:"origin_country"`
}

func (r *tvDetailRaw) toProto(cfg *tmdbConfig) *metadatav1.GetTVDetailsResponse {
	resp := &metadatav1.GetTVDetailsResponse{
		Id:               int32(r.ID),
		Name:             r.Name,
		OriginalName:     r.OriginalName,
		Overview:         r.Overview,
		Tagline:          r.Tagline,
		PosterPath:       r.PosterPath,
		BackdropPath:     r.BackdropPath,
		FirstAirDate:     r.FirstAirDate,
		LastAirDate:      r.LastAirDate,
		NumberOfSeasons:  int32(r.NumSeasons),
		NumberOfEpisodes: int32(r.NumEpisodes),
		VoteAverage:      r.VoteAverage,
		VoteCount:        int32(r.VoteCount),
		Popularity:       r.Popularity,
		Status:           r.Status,
		OriginalLanguage: r.OriginalLanguage,
		Homepage:         r.Homepage,
		InProduction:     strconv.FormatBool(r.InProduction),
		OriginCountry:    r.OriginCountries,
	}
	if cfg != nil {
		resp.PosterUrl = cfg.Images.SecureBaseURL + "original" + r.PosterPath
		resp.BackdropUrl = cfg.Images.SecureBaseURL + "original" + r.BackdropPath
	}
	for _, g := range r.Genres {
		resp.Genres = append(resp.Genres, &metadatav1.Genre{Id: int32(g.ID), Name: g.Name})
	}
	for _, s := range r.Seasons {
		resp.Seasons = append(resp.Seasons, &metadatav1.Season{
			Id: int32(s.ID), Name: s.Name, Overview: s.Overview,
			PosterPath: s.PosterPath, AirDate: s.AirDate,
			SeasonNumber: int32(s.SeasonNumber), EpisodeCount: int32(s.EpisodeCount),
			VoteAverage: s.VoteAverage,
		})
	}
	for _, c := range r.ProductionCos {
		resp.ProductionCompanies = append(resp.ProductionCompanies, &metadatav1.ProductionCompany{
			Id: int32(c.ID), Name: c.Name, LogoPath: c.LogoPath, OriginCountry: c.OriginCountry,
		})
	}
	for _, l := range r.SpokenLangs {
		resp.SpokenLanguages = append(resp.SpokenLanguages, &metadatav1.SpokenLanguage{Iso_639_1: l.ISO6391, Name: l.Name})
	}
	for _, c := range r.CreatedBy {
		resp.CreatedBy = append(resp.CreatedBy, &metadatav1.CreatedBy{Id: int32(c.ID), Name: c.Name, ProfilePath: c.ProfilePath})
	}
	for _, n := range r.Networks {
		resp.Networks = append(resp.Networks, &metadatav1.Network{Id: int32(n.ID), Name: n.Name, LogoPath: n.LogoPath, OriginCountry: n.OriginCountry})
	}
	return resp
}

var _ contracts.Module = (*Module)(nil)
