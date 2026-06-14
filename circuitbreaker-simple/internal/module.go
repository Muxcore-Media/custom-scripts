package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	circuitbreakerv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/circuitbreaker/v1"
)

type circuit struct {
	mu       sync.RWMutex
	state    circuitbreakerv1.CircuitState
	failures uint32

	openedAt      time.Time
	halfOpenAfter time.Duration
}

type Module struct {
	circuitbreakerv1.UnimplementedCircuitBreakerServiceServer

	mu       sync.RWMutex
	circuits map[string]*circuit

	failureThreshold uint32
	cooldown         time.Duration
	halfOpenMax      uint32

	id       string
	grpcAddr string
	grpcSrv  *grpc.Server
	lis      net.Listener
}

type Config struct {
	ID               string
	GRPCAddr         string
	FailureThreshold uint32
	Cooldown         time.Duration
	HalfOpenMax      uint32
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "circuitbreaker-simple"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9640"
	}
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.Cooldown == 0 {
		cfg.Cooldown = 30 * time.Second
	}
	if cfg.HalfOpenMax == 0 {
		cfg.HalfOpenMax = 3
	}
	if v := os.Getenv("CB_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("CB_FAILURE_THRESHOLD"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.FailureThreshold)
	}
	if v := os.Getenv("CB_COOLDOWN"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Cooldown = d
		}
	}
	return &Module{
		id:               cfg.ID,
		grpcAddr:         cfg.GRPCAddr,
		failureThreshold: cfg.FailureThreshold,
		cooldown:         cfg.Cooldown,
		halfOpenMax:      cfg.HalfOpenMax,
		circuits:         make(map[string]*circuit),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Circuit Breaker Simple",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "In-memory circuit breaker with configurable failure threshold and auto-recovery cooldown",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityCircuitBreaker},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "CircuitBreaker",
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
	slog.Info("circuitbreaker-simple initialized",
		"addr", m.grpcAddr,
		"failure_threshold", m.failureThreshold,
		"cooldown", m.cooldown,
		"half_open_max", m.halfOpenMax,
	)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	circuitbreakerv1.RegisterCircuitBreakerServiceServer(m.grpcSrv, m)

	go func() {
		slog.Info("circuitbreaker-simple gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("circuitbreaker-simple gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("circuitbreaker-simple stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) getOrCreate(key string) *circuit {
	m.mu.RLock()
	c, ok := m.circuits[key]
	m.mu.RUnlock()
	if ok {
		return c
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.circuits[key]; ok {
		return c
	}
	c = &circuit{
		state:         circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED,
		halfOpenAfter: m.cooldown,
	}
	m.circuits[key] = c
	return c
}

func (m *Module) State(ctx context.Context, req *circuitbreakerv1.StateRequest) (*circuitbreakerv1.StateResponse, error) {
	c := m.getOrCreate(req.GetKey())
	c.mu.RLock()
	defer c.mu.RUnlock()

	state := m.effectiveState(c)

	resp := &circuitbreakerv1.StateResponse{
		State:        state,
		FailureCount: c.failures,
	}
	if !c.openedAt.IsZero() {
		resp.OpenedAtUnixNano = c.openedAt.UnixNano()
		resp.HalfOpenAfterUnixNano = c.openedAt.Add(c.halfOpenAfter).UnixNano()
	}
	return resp, nil
}

func (m *Module) effectiveState(c *circuit) circuitbreakerv1.CircuitState {
	if c.state == circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN &&
		!c.openedAt.IsZero() &&
		time.Since(c.openedAt) >= c.halfOpenAfter {
		return circuitbreakerv1.CircuitState_CIRCUIT_STATE_HALF_OPEN
	}
	return c.state
}

func (m *Module) RecordSuccess(ctx context.Context, req *circuitbreakerv1.RecordSuccessRequest) (*circuitbreakerv1.RecordSuccessResponse, error) {
	c := m.getOrCreate(req.GetKey())
	c.mu.Lock()
	defer c.mu.Unlock()

	state := m.effectiveState(c)

	switch state {
	case circuitbreakerv1.CircuitState_CIRCUIT_STATE_HALF_OPEN:
		c.state = circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED
		c.failures = 0
		c.openedAt = time.Time{}
	default:
		c.failures = 0
	}

	return &circuitbreakerv1.RecordSuccessResponse{NewState: c.state}, nil
}

func (m *Module) RecordFailure(ctx context.Context, req *circuitbreakerv1.RecordFailureRequest) (*circuitbreakerv1.RecordFailureResponse, error) {
	c := m.getOrCreate(req.GetKey())
	c.mu.Lock()
	defer c.mu.Unlock()

	state := m.effectiveState(c)
	c.failures++
	justOpened := false

	switch state {
	case circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED:
		if c.failures >= m.failureThreshold {
			c.state = circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN
			c.openedAt = time.Now()
			justOpened = true
		}
	case circuitbreakerv1.CircuitState_CIRCUIT_STATE_HALF_OPEN:
		c.state = circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN
		c.openedAt = time.Now()
		justOpened = true
	}

	return &circuitbreakerv1.RecordFailureResponse{
		NewState:          c.state,
		CircuitJustOpened: justOpened,
	}, nil
}

func (m *Module) Reset(ctx context.Context, req *circuitbreakerv1.ResetRequest) (*circuitbreakerv1.ResetResponse, error) {
	c := m.getOrCreate(req.GetKey())
	c.mu.Lock()
	defer c.mu.Unlock()

	prev := c.state
	c.state = circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED
	c.failures = 0
	c.openedAt = time.Time{}

	return &circuitbreakerv1.ResetResponse{PreviousState: prev}, nil
}

var errCircuitOpen = contracts.ErrCircuitOpen

var _ contracts.Module = (*Module)(nil)
