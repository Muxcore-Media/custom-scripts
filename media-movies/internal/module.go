package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/core/sdk/go/client"
	mgmntv1 "github.com/Muxcore-Media/media-movies/proto/mgmntv1"
	_ "modernc.org/sqlite"
)

type Module struct {
	mediaadminv1.UnimplementedMediaAdminServiceServer
	mgmntv1.UnimplementedMovieManagementServiceServer

	mu sync.RWMutex
	db *sql.DB
	mc *client.Client

	id       string
	dbPath   string
	grpcAddr string
	httpAddr string
	imageDir string
	grpcSrv  *grpc.Server
	httpSrv  *http.Server
	grpcLis  net.Listener
	httpLis  net.Listener
}

type Config struct {
	ID       string
	DBPath   string
	GRPCAddr string
	HTTPAddr string
	ImageDir string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "media-movies"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "/var/lib/media-movies/movies.db"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9420"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9430"
	}
	if cfg.ImageDir == "" {
		cfg.ImageDir = "/var/lib/media-movies/images"
	}
	if v := os.Getenv("MOVIES_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("MOVIES_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("MOVIES_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("MOVIES_IMAGE_DIR"); v != "" {
		cfg.ImageDir = v
	}
	return &Module{
		id:       cfg.ID,
		dbPath:   cfg.DBPath,
		grpcAddr: cfg.GRPCAddr,
		httpAddr: cfg.HTTPAddr,
		imageDir: cfg.ImageDir,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Media Movies",
		Version:      "0.1.0",
		Roles:        []string{"media_manager"},
		Description:  "Movie library manager with TMDB metadata import and admin UI integration",
		Author:       "MuxCore",
		Capabilities: []string{"media.library"},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/contracts-media-admin",
				Interface: "MediaAdminService",
				Version:   "v0.1.0",
			},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	dir := filepath.Dir(m.dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
	if err := os.MkdirAll(m.imageDir, 0700); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}

	db, err := sql.Open("sqlite", m.dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS movies (
			id           TEXT PRIMARY KEY,
			tmdb_id      INTEGER UNIQUE,
			title        TEXT NOT NULL,
			original_title TEXT DEFAULT '',
			year         INTEGER DEFAULT 0,
			overview     TEXT DEFAULT '',
			tagline      TEXT DEFAULT '',
			runtime      INTEGER DEFAULT 0,
			vote_average REAL DEFAULT 0,
			status       TEXT DEFAULT '',
			imdb_id      TEXT DEFAULT '',
			genres       TEXT DEFAULT '[]',
			poster_path  TEXT DEFAULT '',
			backdrop_path TEXT DEFAULT '',
			monitored    INTEGER DEFAULT 1,
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)
	`); err != nil {
		db.Close()
		return fmt.Errorf("create movies table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_movies_title ON movies(title)
	`); err != nil {
		db.Close()
		return fmt.Errorf("create index: %w", err)
	}

	m.mu.Lock()
	m.db = db
	m.mu.Unlock()

	grpcLis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		db.Close()
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.grpcLis = grpcLis

	httpLis, err := net.Listen("tcp", m.httpAddr)
	if err != nil {
		db.Close()
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	m.httpLis = httpLis

	slog.Info("media-movies initialized",
		"db", m.dbPath,
		"grpc", m.grpcAddr,
		"http", m.httpAddr,
	)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	mediaadminv1.RegisterMediaAdminServiceServer(m.grpcSrv, m)
	mgmntv1.RegisterMovieManagementServiceServer(m.grpcSrv, m)

	mux := http.NewServeMux()
	mux.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir(m.imageDir))))
	m.httpSrv = &http.Server{Handler: mux}

	go func() {
		slog.Info("media-movies gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("media-movies gRPC serve error", "error", err)
		}
	}()
	go func() {
		slog.Info("media-movies HTTP service started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(m.httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("media-movies HTTP serve error", "error", err)
		}
	}()

	go m.dialCore(context.Background())
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.httpSrv != nil {
		m.httpSrv.Shutdown(ctx)
	}
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.mc != nil {
		m.mc.Close()
	}
	m.mu.Lock()
	if m.db != nil {
		m.db.Close()
		m.db = nil
	}
	m.mu.Unlock()
	slog.Info("media-movies stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("not initialized")
	}
	return db.PingContext(ctx)
}

func (m *Module) dialCore(ctx context.Context) {
	meshAddr := os.Getenv("MUXCORE_GRPC_ADDR")
	if meshAddr == "" {
		meshAddr = "localhost:9090"
	}
	insecureMode := os.Getenv("MUXCORE_GRPC_INSECURE") == "true"

	var opts []client.Option
	if insecureMode {
		opts = append(opts, client.WithInsecure())
	}

	c, err := client.Dial(meshAddr, opts...)
	if err != nil {
		slog.Error("media-movies: dial core", "error", err)
		return
	}
	m.mc = c
	slog.Info("media-movies: connected to core mesh", "addr", meshAddr)
}

func (m *Module) findMetadataModule(ctx context.Context) (string, error) {
	if m.mc == nil {
		return "", fmt.Errorf("not connected to core")
	}
	modules, err := m.mc.Discovery.FindByCapability(ctx, "metadata")
	if err != nil {
		return "", fmt.Errorf("discover metadata: %w", err)
	}
	for _, mod := range modules {
		if mod.HttpAddr != "" {
			return mod.HttpAddr, nil
		}
	}
	return "", fmt.Errorf("no metadata module found")
}

// ── MovieManagementService ────────────────────────────────────

func (m *Module) AddMovie(ctx context.Context, req *mgmntv1.AddMovieRequest) (*mgmntv1.AddMovieResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("mv_%d_%s", req.GetTmdbId(), now)

	genresJSON, _ := json.Marshal(req.GetGenres())

	_, err := m.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO movies (id, tmdb_id, title, year, overview, poster_path, backdrop_path, genres, monitored, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, req.GetTmdbId(), req.GetTitle(), req.GetYear(),
		req.GetOverview(), req.GetPosterPath(), req.GetBackdropPath(),
		string(genresJSON), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert movie: %w", err)
	}

	return &mgmntv1.AddMovieResponse{MovieId: id}, nil
}

func (m *Module) RemoveMovie(ctx context.Context, req *mgmntv1.RemoveMovieRequest) (*mgmntv1.RemoveMovieResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}
	_, err := m.db.ExecContext(ctx, `DELETE FROM movies WHERE id = ?`, req.GetMovieId())
	if err != nil {
		return nil, fmt.Errorf("delete movie: %w", err)
	}
	return &mgmntv1.RemoveMovieResponse{}, nil
}

func (m *Module) RefreshMetadata(ctx context.Context, req *mgmntv1.RefreshMetadataRequest) (*mgmntv1.RefreshMetadataResponse, error) {
	metaAddr, err := m.findMetadataModule(ctx)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(metaAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial metadata: %w", err)
	}
	defer conn.Close()

	// TODO: implement metadata refresh via metadata-tmdb gRPC
	slog.Info("refresh metadata requested", "movie_id", req.GetMovieId(), "metadata_addr", metaAddr)
	return &mgmntv1.RefreshMetadataResponse{}, nil
}

func (m *Module) ListMovies(ctx context.Context, req *mgmntv1.ListMoviesRequest) (*mgmntv1.ListMoviesResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `SELECT id, tmdb_id, title, original_title, year, overview, tagline,
		runtime, vote_average, status, imdb_id, genres, poster_path, backdrop_path,
		monitored, created_at, updated_at FROM movies`
	countQuery := `SELECT COUNT(*) FROM movies`

	var args []any
	var where []string

	if req.GetSearch() != "" {
		where = append(where, `title LIKE ?`)
		args = append(args, "%"+req.GetSearch()+"%")
	}
	if req.GetGenre() != "" {
		where = append(where, `genres LIKE ?`)
		args = append(args, `%"`+req.GetGenre()+`"%`)
	}
	if len(where) > 0 {
		clause := ` WHERE ` + strings.Join(where, ` AND `)
		query += clause
		countQuery += clause
	}

	var total int
	m.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)

	sortBy := req.GetSortBy()
	if sortBy == "" {
		sortBy = "title"
	}
	sortOrder := req.GetSortOrder()
	if sortOrder != "desc" {
		sortOrder = "asc"
	}
	query += fmt.Sprintf(` ORDER BY %s %s LIMIT ? OFFSET ?`, sortBy, sortOrder)
	queryArgs := append(args, pageSize, offset)

	rows, err := m.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query movies: %w", err)
	}
	defer rows.Close()

	var movies []*mgmntv1.MovieItem
	for rows.Next() {
		movie := m.scanMovie(rows)
		if movie != nil {
			movies = append(movies, movie)
		}
	}

	return &mgmntv1.ListMoviesResponse{
		Movies:   movies,
		Total:    int32(total),
		Page:     int32(page),
		PageSize: int32(pageSize),
	}, nil
}

func (m *Module) GetMovie(ctx context.Context, req *mgmntv1.GetMovieRequest) (*mgmntv1.GetMovieResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	row := m.db.QueryRowContext(ctx,
		`SELECT id, tmdb_id, title, original_title, year, overview, tagline,
		 runtime, vote_average, status, imdb_id, genres, poster_path, backdrop_path,
		 monitored, created_at, updated_at FROM movies WHERE id = ?`,
		req.GetMovieId(),
	)

	movie := m.scanSingle(row)
	if movie == nil {
		return nil, fmt.Errorf("movie not found: %s", req.GetMovieId())
	}
	return &mgmntv1.GetMovieResponse{Movie: movie}, nil
}

func (m *Module) scanMovie(rows *sql.Rows) *mgmntv1.MovieItem {
	var id, title, originalTitle, overview, tagline, status, imdbID, genresStr, posterPath, backdropPath, createdAt, updatedAt string
	var tmdbID, year, runtime int64
	var voteAvg float64
	var monitored int

	err := rows.Scan(&id, &tmdbID, &title, &originalTitle, &year, &overview, &tagline,
		&runtime, &voteAvg, &status, &imdbID, &genresStr, &posterPath, &backdropPath,
		&monitored, &createdAt, &updatedAt)
	if err != nil {
		slog.Error("scan movie row", "error", err)
		return nil
	}

	var genres []string
	json.Unmarshal([]byte(genresStr), &genres)
	if genres == nil {
		genres = []string{}
	}

	return &mgmntv1.MovieItem{
		Id: id, TmdbId: int32(tmdbID),
		Title: title, OriginalTitle: originalTitle,
		Year: int32(year), Overview: overview, Tagline: tagline,
		Runtime: int32(runtime), VoteAverage: voteAvg,
		Status: status, ImdbId: imdbID,
		Genres: genres, PosterPath: posterPath, BackdropPath: backdropPath,
		Monitored: monitored != 0,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

func (m *Module) scanSingle(row *sql.Row) *mgmntv1.MovieItem {
	var id, title, originalTitle, overview, tagline, status, imdbID, genresStr, posterPath, backdropPath, createdAt, updatedAt string
	var tmdbID, year, runtime int64
	var voteAvg float64
	var monitored int

	err := row.Scan(&id, &tmdbID, &title, &originalTitle, &year, &overview, &tagline,
		&runtime, &voteAvg, &status, &imdbID, &genresStr, &posterPath, &backdropPath,
		&monitored, &createdAt, &updatedAt)
	if err != nil {
		return nil
	}

	var genres []string
	json.Unmarshal([]byte(genresStr), &genres)
	if genres == nil {
		genres = []string{}
	}

	return &mgmntv1.MovieItem{
		Id: id, TmdbId: int32(tmdbID),
		Title: title, OriginalTitle: originalTitle,
		Year: int32(year), Overview: overview, Tagline: tagline,
		Runtime: int32(runtime), VoteAverage: voteAvg,
		Status: status, ImdbId: imdbID,
		Genres: genres, PosterPath: posterPath, BackdropPath: backdropPath,
		Monitored: monitored != 0,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

func movieToMediaItem(m *mgmntv1.MovieItem) *mediaadminv1.MediaItem {
	meta := map[string]string{
		"tmdb_id":  strconv.Itoa(int(m.GetTmdbId())),
		"runtime":  strconv.Itoa(int(m.GetRuntime())),
		"vote_avg": fmt.Sprintf("%.1f", m.GetVoteAverage()),
		"imdb_id":  m.GetImdbId(),
		"status":   m.GetStatus(),
	}
	if m.GetTagline() != "" {
		meta["tagline"] = m.GetTagline()
	}

	return &mediaadminv1.MediaItem{
		Id:          m.GetId(),
		Title:       m.GetTitle(),
		Description: m.GetOverview(),
		Year:        int64(m.GetYear()),
		Genres:      m.GetGenres(),
		Metadata:    meta,
		CreatedAt:   m.GetCreatedAt(),
		UpdatedAt:   m.GetUpdatedAt(),
	}
}

// ── MediaAdminService (admin-ui contract) ─────────────────────

func (m *Module) GetMediaTypeInfo(ctx context.Context, req *mediaadminv1.GetMediaTypeInfoRequest) (*mediaadminv1.GetMediaTypeInfoResponse, error) {
	return &mediaadminv1.GetMediaTypeInfoResponse{
		DisplayName: "Movies",
		Icon:        "🎬",
		FilterFields: []*mediaadminv1.FilterField{
			{Key: "genre", Label: "Genre", Type: "text"},
			{Key: "year", Label: "Year", Type: "number"},
			{Key: "runtime", Label: "Runtime", Type: "number"},
		},
	}, nil
}

func (m *Module) ListItems(ctx context.Context, req *mediaadminv1.ListItemsRequest) (*mediaadminv1.ListItemsResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `SELECT id, tmdb_id, title, original_title, year, overview, tagline,
		runtime, vote_average, status, imdb_id, genres, poster_path, backdrop_path,
		monitored, created_at, updated_at FROM movies`
	countQuery := `SELECT COUNT(*) FROM movies`

	var args []any
	var where []string

	if req.GetSearch() != "" {
		where = append(where, `title LIKE ?`)
		args = append(args, "%"+req.GetSearch()+"%")
	}

	sortBy := req.GetSortBy()
	if sortBy == "" {
		sortBy = "title"
	}
	sortOrder := req.GetSortOrder()
	if sortOrder != "desc" {
		sortOrder = "asc"
	}

	if len(where) > 0 {
		clause := ` WHERE ` + strings.Join(where, ` AND `)
		query += clause
		countQuery += clause
	}

	var total int
	m.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)

	query += fmt.Sprintf(` ORDER BY %s %s LIMIT ? OFFSET ?`, sortBy, sortOrder)
	qargs := append(args, pageSize, offset)

	rows, err := m.db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	var items []*mediaadminv1.MediaItem
	for rows.Next() {
		movie := m.scanMovie(rows)
		if movie != nil {
			items = append(items, movieToMediaItem(movie))
		}
	}

	return &mediaadminv1.ListItemsResponse{
		Items:    items,
		Total:    int32(total),
		Page:     int32(page),
		PageSize: int32(pageSize),
	}, nil
}

func (m *Module) GetItem(ctx context.Context, req *mediaadminv1.GetItemRequest) (*mediaadminv1.GetItemResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	row := m.db.QueryRowContext(ctx,
		`SELECT id, tmdb_id, title, original_title, year, overview, tagline,
		 runtime, vote_average, status, imdb_id, genres, poster_path, backdrop_path,
		 monitored, created_at, updated_at FROM movies WHERE id = ?`,
		req.GetId(),
	)

	movie := m.scanSingle(row)
	if movie == nil {
		return nil, fmt.Errorf("movie not found: %s", req.GetId())
	}
	return &mediaadminv1.GetItemResponse{Item: movieToMediaItem(movie)}, nil
}

func (m *Module) UpdateMetadata(ctx context.Context, req *mediaadminv1.UpdateMetadataRequest) (*mediaadminv1.UpdateMetadataResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	genresJSON, _ := json.Marshal(req.GetGenres())

	_, err := m.db.ExecContext(ctx,
		`UPDATE movies SET title=?, overview=?, year=?, genres=?, updated_at=? WHERE id=?`,
		req.GetTitle(), req.GetDescription(), req.GetYear(),
		string(genresJSON), now, req.GetId(),
	)
	if err != nil {
		return nil, fmt.Errorf("update movie: %w", err)
	}

	row := m.db.QueryRowContext(ctx,
		`SELECT id, tmdb_id, title, original_title, year, overview, tagline,
		 runtime, vote_average, status, imdb_id, genres, poster_path, backdrop_path,
		 monitored, created_at, updated_at FROM movies WHERE id = ?`,
		req.GetId(),
	)
	movie := m.scanSingle(row)
	if movie == nil {
		return nil, fmt.Errorf("movie not found after update: %s", req.GetId())
	}
	return &mediaadminv1.UpdateMetadataResponse{Item: movieToMediaItem(movie)}, nil
}

func (m *Module) ListArtwork(ctx context.Context, req *mediaadminv1.ListArtworkRequest) (*mediaadminv1.ListArtworkResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	row := m.db.QueryRowContext(ctx,
		`SELECT poster_path, backdrop_path FROM movies WHERE id = ?`, req.GetId(),
	)
	var poster, backdrop string
	if err := row.Scan(&poster, &backdrop); err != nil {
		return nil, fmt.Errorf("movie not found: %s", req.GetId())
	}

	var artwork []*mediaadminv1.ArtworkInfo
	if poster != "" {
		artwork = append(artwork, &mediaadminv1.ArtworkInfo{
			Id:     req.GetId() + "_poster",
			ItemId: req.GetId(),
			Type:   "poster",
			Url:    fmt.Sprintf("http://%s/images/%s", m.httpAddr, poster),
		})
	}
	if backdrop != "" {
		artwork = append(artwork, &mediaadminv1.ArtworkInfo{
			Id:     req.GetId() + "_backdrop",
			ItemId: req.GetId(),
			Type:   "background",
			Url:    fmt.Sprintf("http://%s/images/%s", m.httpAddr, backdrop),
		})
	}

	return &mediaadminv1.ListArtworkResponse{Artwork: artwork}, nil
}

func (m *Module) ReplaceArtwork(stream grpc.ClientStreamingServer[mediaadminv1.ReplaceArtworkRequest, mediaadminv1.ReplaceArtworkResponse]) error {
	slog.Warn("ReplaceArtwork not yet implemented")
	return nil
}

var _ contracts.Module = (*Module)(nil)
