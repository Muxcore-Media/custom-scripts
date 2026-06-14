package internal

import (
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
	dataredactionv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/dataredaction/v1"
)

const redactedString = "***REDACTED***"

type ruleKind int

const (
	ruleFieldPrefix ruleKind = iota
	rulePath
	ruleRegex
)

type compiledRule struct {
	kind ruleKind
	raw  string
	re   *regexp.Regexp
	path string // for path rules, without "path:" prefix
}

type Module struct {
	dataredactionv1.UnimplementedDataRedactionServiceServer

	mu          sync.RWMutex
	defaultRe   []*regexp.Regexp
	defaultKeys []string

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
		cfg.ID = "data-redaction-pattern"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9650"
	}
	if v := os.Getenv("REDACTION_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}

	mod := &Module{
		id:       cfg.ID,
		grpcAddr: cfg.GRPCAddr,
	}

	for _, key := range contracts.SensitiveLogFieldNames() {
		mod.defaultKeys = append(mod.defaultKeys, strings.ToLower(key))
	}

	return mod
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Data Redaction Pattern",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "Field-name prefix, path, and regex based PII redaction provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityDataRedaction},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "DataRedactionProvider",
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
	slog.Info("data-redaction-pattern initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	dataredactionv1.RegisterDataRedactionServiceServer(m.grpcSrv, m)

	go func() {
		slog.Info("data-redaction-pattern gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("data-redaction-pattern gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("data-redaction-pattern stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) Redact(ctx context.Context, req *dataredactionv1.RedactRequest) (*dataredactionv1.RedactResponse, error) {
	var data map[string]any
	if err := json.Unmarshal(req.GetData(), &data); err != nil {
		return nil, fmt.Errorf("decode input data: %w", err)
	}

	rules, err := compileRules(req.GetRules())
	if err != nil {
		return nil, fmt.Errorf("compile rules: %w", err)
	}

	redacted := redactMap(data, m.defaultKeys, rules)

	out, err := json.Marshal(redacted)
	if err != nil {
		return nil, fmt.Errorf("encode redacted data: %w", err)
	}

	return &dataredactionv1.RedactResponse{Data: out}, nil
}

func (m *Module) SupportedRules(ctx context.Context, req *dataredactionv1.SupportedRulesRequest) (*dataredactionv1.SupportedRulesResponse, error) {
	return &dataredactionv1.SupportedRulesResponse{
		RuleTypes: []string{
			"field:<name>     — redact keys containing <name> (case-insensitive)",
			"path:<a.b.c>     — redact nested path a → b → c",
			"/<regex>/        — redact values matching <regex>",
		},
	}, nil
}

func compileRules(raw []string) ([]compiledRule, error) {
	var rules []compiledRule
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}

		if strings.HasPrefix(r, "field:") {
			rules = append(rules, compiledRule{
				kind: ruleFieldPrefix,
				raw:  strings.ToLower(strings.TrimPrefix(r, "field:")),
			})
		} else if strings.HasPrefix(r, "path:") {
			path := strings.TrimPrefix(r, "path:")
			rules = append(rules, compiledRule{
				kind: rulePath,
				raw:  r,
				path: path,
			})
		} else if len(r) > 2 && r[0] == '/' && strings.LastIndex(r, "/") > 0 {
			inner := r[1 : len(r)-1]
			re, err := regexp.Compile(inner)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %w", r, err)
			}
			rules = append(rules, compiledRule{
				kind: ruleRegex,
				raw:  r,
				re:   re,
			})
		} else {
			rules = append(rules, compiledRule{
				kind: ruleFieldPrefix,
				raw:  strings.ToLower(r),
			})
		}
	}
	return rules, nil
}

func redactMap(data map[string]any, defaultKeys []string, rules []compiledRule) map[string]any {
	return redactMapWithPath(data, defaultKeys, rules, "")
}

func redactMapWithPath(data map[string]any, defaultKeys []string, rules []compiledRule, parentPath string) map[string]any {
	out := make(map[string]any, len(data))
	for k, v := range data {
		keyLower := strings.ToLower(k)
		currentPath := k
		if parentPath != "" {
			currentPath = parentPath + "." + k
		}

		redact := false

		for _, dk := range defaultKeys {
			if strings.Contains(keyLower, dk) {
				redact = true
				break
			}
		}
		if !redact {
			for _, rule := range rules {
				if matchesRule(k, v, rule, currentPath) {
					redact = true
					break
				}
			}
		}

		if redact {
			out[k] = redactedString
		} else {
			out[k] = redactValueWithPath(v, defaultKeys, rules, currentPath)
		}
	}
	return out
}

func matchesRule(key string, val any, rule compiledRule, currentPath string) bool {
	switch rule.kind {
	case ruleFieldPrefix:
		return strings.Contains(strings.ToLower(key), rule.raw)
	case rulePath:
		return currentPath == rule.path
	case ruleRegex:
		s, ok := val.(string)
		if !ok {
			return false
		}
		return rule.re.MatchString(s)
	}
	return false
}

func redactValue(v any, defaultKeys []string, rules []compiledRule) any {
	return redactValueWithPath(v, defaultKeys, rules, "")
}

func redactValueWithPath(v any, defaultKeys []string, rules []compiledRule, parentPath string) any {
	switch val := v.(type) {
	case map[string]any:
		return redactMapWithPath(val, defaultKeys, rules, parentPath)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			if m, ok := item.(map[string]any); ok {
				out[i] = redactMapWithPath(m, defaultKeys, rules, parentPath)
			} else {
				out[i] = item
			}
		}
		return out
	default:
		return v
	}
}

var _ contracts.Module = (*Module)(nil)
