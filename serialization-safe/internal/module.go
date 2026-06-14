package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	serializationv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/serialization/v1"
	"github.com/vmihailenco/msgpack/v5"
)

var (
	supportedTypes = []string{
		contracts.SafeContentTypeJSON,
		contracts.SafeContentTypeMsgpack,
	}
	errUnsupportedType       = errors.New("unsupported content type")
	errUnsupportedConversion = errors.New("unsupported conversion between source and target types")
)

type Module struct {
	serializationv1.UnimplementedSerializationServiceServer

	mu       sync.RWMutex
	id       string
	grpcAddr string
	grpcSrv  *grpc.Server
	lis      net.Listener
}

type Config struct {
	ID       string
	GRPCAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "serialization-safe"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9630"
	}
	if v := os.Getenv("SERIALIZATION_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	return &Module{
		id:       cfg.ID,
		grpcAddr: cfg.GRPCAddr,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Serialization Safe",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "Safe content-type serialization provider supporting JSON and msgpack bidirectional conversion",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilitySerialization},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "SerializationProvider",
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
	slog.Info("serialization-safe initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	serializationv1.RegisterSerializationServiceServer(m.grpcSrv, m)

	go func() {
		slog.Info("serialization-safe gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("serialization-safe gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("serialization-safe stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) Convert(ctx context.Context, req *serializationv1.ConvertRequest) (*serializationv1.ConvertResponse, error) {
	src := req.GetSourceContentType()
	tgt := req.GetTargetContentType()
	data := req.GetData()

	if !isSupported(src) {
		return nil, fmt.Errorf("%w: %s", errUnsupportedType, src)
	}
	if !isSupported(tgt) {
		return nil, fmt.Errorf("%w: %s", errUnsupportedType, tgt)
	}

	if src == tgt {
		return &serializationv1.ConvertResponse{Result: data}, nil
	}

	result, err := convert(src, tgt, data)
	if err != nil {
		return nil, err
	}
	return &serializationv1.ConvertResponse{Result: result}, nil
}

func (m *Module) SupportedTypes(ctx context.Context, req *serializationv1.SupportedTypesRequest) (*serializationv1.SupportedTypesResponse, error) {
	return &serializationv1.SupportedTypesResponse{
		ContentTypes: supportedTypes,
	}, nil
}

func convert(src, tgt string, data []byte) ([]byte, error) {
	if src == tgt {
		return data, nil
	}

	switch {
	case src == contracts.SafeContentTypeJSON && tgt == contracts.SafeContentTypeMsgpack:
		return jsonToMsgpack(data)
	case src == contracts.SafeContentTypeMsgpack && tgt == contracts.SafeContentTypeJSON:
		return msgpackToJSON(data)
	default:
		return nil, fmt.Errorf("%w: %s -> %s", errUnsupportedConversion, src, tgt)
	}
}

func jsonToMsgpack(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	out, err := msgpack.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("msgpack encode: %w", err)
	}
	return out, nil
}

func msgpackToJSON(data []byte) ([]byte, error) {
	var v any
	if err := msgpack.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("msgpack decode: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json encode: %w", err)
	}
	return out, nil
}

func isSupported(ct string) bool {
	for _, s := range supportedTypes {
		if s == ct {
			return true
		}
	}
	return false
}

var _ contracts.Module = (*Module)(nil)
