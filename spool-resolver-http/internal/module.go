package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	spoolresolverv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/spoolresolver/v1"
)

type Module struct {
	spoolresolverv1.UnimplementedSpoolResolverServiceServer

	mu       sync.RWMutex
	client   *http.Client
	id       string
	grpcAddr string
	grpcSrv  *grpc.Server
	lis      net.Listener
}

type Config struct {
	ID       string
	GRPCAddr string
	Timeout  time.Duration
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "spool-resolver-http"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9670"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if v := os.Getenv("SPOOL_RESOLVER_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	return &Module{
		id:       cfg.ID,
		grpcAddr: cfg.GRPCAddr,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Spool Resolver HTTP",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "HTTPS JSON spool tag resolver provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilitySpoolResolver},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "SpoolResolver",
				Version:   "v0.4.0",
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
	slog.Info("spool-resolver-http initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	spoolresolverv1.RegisterSpoolResolverServiceServer(m.grpcSrv, m)
	go func() {
		slog.Info("spool-resolver-http gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("spool-resolver-http gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	m.client.CloseIdleConnections()
	slog.Info("spool-resolver-http stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) ResolveTag(ctx context.Context, req *spoolresolverv1.ResolveTagRequest) (*spoolresolverv1.ResolveTagResponse, error) {
	url := fmt.Sprintf("%s/%s", req.GetSpoolUrl(), req.GetTagName())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &spoolresolverv1.ResolveTagResponse{
			Error: fmt.Sprintf("create request: %v", err),
		}, nil
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return &spoolresolverv1.ResolveTagResponse{
			Error: fmt.Sprintf("fetch tag: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &spoolresolverv1.ResolveTagResponse{
			Error: fmt.Sprintf("spool returned %s", resp.Status),
		}, nil
	}

	var tag contracts.TagDefinition
	if err := json.NewDecoder(resp.Body).Decode(&tag); err != nil {
		return &spoolresolverv1.ResolveTagResponse{
			Error: fmt.Sprintf("decode tag: %v", err),
		}, nil
	}

	raw, err := json.Marshal(tag)
	if err != nil {
		return &spoolresolverv1.ResolveTagResponse{
			Error: fmt.Sprintf("marshal tag: %v", err),
		}, nil
	}

	return &spoolresolverv1.ResolveTagResponse{TagDefinitionJson: raw}, nil
}

var _ contracts.Module = (*Module)(nil)
