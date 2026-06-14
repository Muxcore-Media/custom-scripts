package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	inputvalidatev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/inputvalidate/v1"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

type Module struct {
	inputvalidatev1.UnimplementedInputValidateServiceServer

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
		cfg.ID = "input-validate-jsonschema"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9660"
	}
	if v := os.Getenv("VALIDATE_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	return &Module{id: cfg.ID, grpcAddr: cfg.GRPCAddr}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Input Validate JSON Schema",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "JSON Schema and regex based input validation provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityInputValidate},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "InputValidator",
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
	slog.Info("input-validate-jsonschema initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	inputvalidatev1.RegisterInputValidateServiceServer(m.grpcSrv, m)
	go func() {
		slog.Info("input-validate-jsonschema gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("input-validate-jsonschema gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("input-validate-jsonschema stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) Validate(ctx context.Context, req *inputvalidatev1.ValidateRequest) (*inputvalidatev1.ValidateResponse, error) {
	schema := req.GetSchema()
	data := req.GetData()

	if strings.HasPrefix(schema, "json:") {
		return m.validateJSON(schema, data)
	}
	if strings.HasPrefix(schema, "regex:") {
		return m.validateRegex(schema, data)
	}
	return &inputvalidatev1.ValidateResponse{
		Valid:  false,
		Errors: []string{fmt.Sprintf("unsupported schema type: %q", schema)},
	}, nil
}

func (m *Module) validateJSON(schema string, data []byte) (*inputvalidatev1.ValidateResponse, error) {
	schemaName := strings.TrimPrefix(schema, "json:")
	schemaName = strings.TrimSpace(schemaName)
	if schemaName == "" {
		return &inputvalidatev1.ValidateResponse{
			Valid:  false,
			Errors: []string{"json schema name is empty"},
		}, nil
	}

	var compiledSchema *jsonschema.Schema
	var err error
	if strings.HasPrefix(schemaName, "{") {
		compiledSchema, err = jsonschema.CompileString("schema.json", schemaName)
	} else {
		compiledSchema, err = jsonschema.CompileString(schemaName+".json", schemaName)
	}
	_ = compiledSchema
	if err != nil {
		return nil, fmt.Errorf("compile json schema: %w", err)
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return &inputvalidatev1.ValidateResponse{
			Valid:  false,
			Errors: []string{fmt.Sprintf("input is not valid JSON: %v", err)},
		}, nil
	}

	if err := compiledSchema.Validate(v); err != nil {
		var ve *jsonschema.ValidationError
		if ok := asValidationError(err, &ve); ok {
			errs := make([]string, 0, len(ve.Causes))
			for _, c := range ve.Causes {
				errs = append(errs, fmt.Sprintf("%s: %s", c.InstanceLocation, c.Message))
			}
			return &inputvalidatev1.ValidateResponse{
				Valid:  false,
				Errors: errs,
			}, nil
		}
		return &inputvalidatev1.ValidateResponse{
			Valid:  false,
			Errors: []string{err.Error()},
		}, nil
	}

	return &inputvalidatev1.ValidateResponse{
		Valid:     true,
		Sanitized: data,
	}, nil
}

func (m *Module) validateRegex(schema string, data []byte) (*inputvalidatev1.ValidateResponse, error) {
	pattern := strings.TrimPrefix(schema, "regex:")
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compile regex: %w", err)
	}

	str := string(bytes.TrimSpace(data))
	if re.MatchString(str) {
		return &inputvalidatev1.ValidateResponse{
			Valid:     true,
			Sanitized: []byte(str),
		}, nil
	}

	return &inputvalidatev1.ValidateResponse{
		Valid:  false,
		Errors: []string{fmt.Sprintf("input %q does not match pattern %q", str, pattern)},
	}, nil
}

func (m *Module) SupportedSchemas(ctx context.Context, req *inputvalidatev1.SupportedSchemasRequest) (*inputvalidatev1.SupportedSchemasResponse, error) {
	return &inputvalidatev1.SupportedSchemasResponse{
		Schemas: []string{"json:<schema>", "regex:<pattern>"},
	}, nil
}

func asValidationError(err error, target **jsonschema.ValidationError) bool {
	for err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			*target = ve
			return true
		}
		err = errorsUnwrap(err)
	}
	return false
}

func errorsUnwrap(err error) error {
	type unwrapper interface {
		Unwrap() error
	}
	u, ok := err.(unwrapper)
	if !ok {
		return nil
	}
	return u.Unwrap()
}

var _ contracts.Module = (*Module)(nil)
