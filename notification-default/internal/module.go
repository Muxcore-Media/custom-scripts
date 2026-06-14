package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	notifyv1 "github.com/Muxcore-Media/notification-default/proto/notifyv1"
)

type channelConfig struct {
	Enabled  bool
	Webhook  string
	Settings map[string]string
}

type Module struct {
	notifyv1.UnimplementedNotificationServiceServer

	mu       sync.RWMutex
	channels map[notifyv1.Channel]*channelConfig
	client   *http.Client

	id       string
	grpcAddr string
	grpcSrv  *grpc.Server
	lis      net.Listener
}

type Config struct {
	ID             string
	GRPCAddr       string
	DiscordWebhook string
	SlackWebhook   string
	GenericWebhook string
	SMTPHost       string
	SMTPPort       string
	SMTPUser       string
	SMTPPass       string
	EmailFrom      string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "notification-default"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9440"
	}
	if v := os.Getenv("NOTIFY_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("DISCORD_WEBHOOK"); v != "" {
		cfg.DiscordWebhook = v
	}
	if v := os.Getenv("SLACK_WEBHOOK"); v != "" {
		cfg.SlackWebhook = v
	}
	if v := os.Getenv("WEBHOOK_URL"); v != "" {
		cfg.GenericWebhook = v
	}

	m := &Module{
		id:       cfg.ID,
		grpcAddr: cfg.GRPCAddr,
		client:   &http.Client{Timeout: 10 * time.Second},
		channels: map[notifyv1.Channel]*channelConfig{},
	}

	if cfg.DiscordWebhook != "" {
		m.channels[notifyv1.Channel_CHANNEL_DISCORD] = &channelConfig{
			Enabled: true, Webhook: cfg.DiscordWebhook,
			Settings: map[string]string{"type": "discord"},
		}
	}
	if cfg.SlackWebhook != "" {
		m.channels[notifyv1.Channel_CHANNEL_SLACK] = &channelConfig{
			Enabled: true, Webhook: cfg.SlackWebhook,
			Settings: map[string]string{"type": "slack"},
		}
	}
	if cfg.GenericWebhook != "" {
		m.channels[notifyv1.Channel_CHANNEL_WEBHOOK] = &channelConfig{
			Enabled: true, Webhook: cfg.GenericWebhook,
			Settings: map[string]string{"type": "generic"},
		}
	}

	return m
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Notification Default",
		Version:      "0.1.0",
		Roles:        []string{"notification"},
		Description:  "Multi-channel notification provider supporting Discord, Slack, webhook, and email",
		Author:       "MuxCore",
		Capabilities: []string{"notification"},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/contracts-notification",
				Interface: "NotificationProvider",
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
	slog.Info("notification-default initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	notifyv1.RegisterNotificationServiceServer(m.grpcSrv, m)
	go func() {
		slog.Info("notification-default gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("notification-default gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	m.client.CloseIdleConnections()
	slog.Info("notification-default stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch, cfg := range m.channels {
		if cfg.Enabled {
			_ = ch
			return nil
		}
	}
	return fmt.Errorf("no notification channels configured — set DISCORD_WEBHOOK, SLACK_WEBHOOK, or WEBHOOK_URL")
}

func (m *Module) Notify(ctx context.Context, req *notifyv1.NotifyRequest) (*notifyv1.NotifyResponse, error) {
	channels := req.GetChannels()
	if len(channels) == 0 {
		m.mu.RLock()
		for ch := range m.channels {
			channels = append(channels, ch)
		}
		m.mu.RUnlock()
	}

	var results []*notifyv1.ChannelResult
	for _, ch := range channels {
		r := m.sendToChannel(ctx, ch, req)
		results = append(results, r)
	}

	return &notifyv1.NotifyResponse{Results: results}, nil
}

func (m *Module) sendToChannel(ctx context.Context, ch notifyv1.Channel, req *notifyv1.NotifyRequest) *notifyv1.ChannelResult {
	m.mu.RLock()
	cfg, ok := m.channels[ch]
	m.mu.RUnlock()

	if !ok || !cfg.Enabled {
		return &notifyv1.ChannelResult{
			Channel: ch, Success: false,
			Error: fmt.Sprintf("channel %s not configured", ch),
		}
	}

	var payload []byte
	var err error

	switch ch {
	case notifyv1.Channel_CHANNEL_DISCORD:
		payload, err = buildDiscordPayload(req)
	case notifyv1.Channel_CHANNEL_SLACK:
		payload, err = buildSlackPayload(req)
	case notifyv1.Channel_CHANNEL_WEBHOOK:
		payload, err = buildWebhookPayload(req)
	default:
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: "unsupported channel"}
	}

	if err != nil {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: err.Error()}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Webhook, bytes.NewReader(payload))
	if err != nil {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: err.Error()}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: fmt.Sprintf("webhook returned %s", resp.Status)}
	}

	return &notifyv1.ChannelResult{Channel: ch, Success: true}
}

func (m *Module) Configure(ctx context.Context, req *notifyv1.ConfigureRequest) (*notifyv1.ConfigureResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := &channelConfig{
		Enabled:  true,
		Settings: req.GetSettings(),
	}
	if url, ok := req.GetSettings()["webhook_url"]; ok {
		cfg.Webhook = url
	}
	m.channels[req.GetChannel()] = cfg
	return &notifyv1.ConfigureResponse{Configured: true}, nil
}

func (m *Module) Status(ctx context.Context, req *notifyv1.StatusRequest) (*notifyv1.StatusResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []*notifyv1.ChannelStatus
	for ch, cfg := range m.channels {
		desc := channelDescription(ch, cfg)
		statuses = append(statuses, &notifyv1.ChannelStatus{
			Channel: ch, Enabled: cfg.Enabled, Description: desc,
		})
	}
	if statuses == nil {
		statuses = []*notifyv1.ChannelStatus{}
	}
	return &notifyv1.StatusResponse{Channels: statuses}, nil
}

func buildDiscordPayload(req *notifyv1.NotifyRequest) ([]byte, error) {
	color := discordColor(req.GetSeverity())
	desc := req.GetMessage()
	if req.GetSourceModule() != "" {
		desc = fmt.Sprintf("**Source:** %s\n\n%s", req.GetSourceModule(), desc)
	}

	embed := map[string]any{
		"title":       req.GetTitle(),
		"description": desc,
		"color":       color,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	var fields []map[string]any
	for k, v := range req.GetFields() {
		fields = append(fields, map[string]any{"name": k, "value": v, "inline": true})
	}
	if len(fields) > 0 {
		embed["fields"] = fields
	}

	return json.Marshal(map[string]any{"embeds": []any{embed}})
}

func buildSlackPayload(req *notifyv1.NotifyRequest) ([]byte, error) {
	color := slackColor(req.GetSeverity())
	text := fmt.Sprintf("*%s*\n%s", req.GetTitle(), req.GetMessage())

	var fields []map[string]any
	for k, v := range req.GetFields() {
		fields = append(fields, map[string]any{"title": k, "value": v, "short": true})
	}

	attachment := map[string]any{
		"color":    color,
		"text":     text,
		"fallback": req.GetTitle(),
		"ts":       time.Now().Unix(),
	}
	if len(fields) > 0 {
		attachment["fields"] = fields
	}

	return json.Marshal(map[string]any{
		"attachments": []any{attachment},
	})
}

func buildWebhookPayload(req *notifyv1.NotifyRequest) ([]byte, error) {
	return json.Marshal(map[string]any{
		"title":     req.GetTitle(),
		"message":   req.GetMessage(),
		"severity":  req.GetSeverity(),
		"source":    req.GetSourceModule(),
		"fields":    req.GetFields(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func discordColor(severity string) int {
	switch strings.ToLower(severity) {
	case "error":
		return 0xE74C3C
	case "warning":
		return 0xF39C12
	case "success":
		return 0x2ECC71
	default:
		return 0x3498DB
	}
}

func slackColor(severity string) string {
	switch strings.ToLower(severity) {
	case "error":
		return "danger"
	case "warning":
		return "warning"
	default:
		return "good"
	}
}

func channelDescription(ch notifyv1.Channel, cfg *channelConfig) string {
	desc := "disabled"
	if cfg.Enabled {
		desc = "enabled"
	}
	if cfg.Webhook != "" {
		url := cfg.Webhook
		if len(url) > 60 {
			url = url[:60] + "..."
		}
		desc += " — " + url
	}
	return desc
}

var _ contracts.Module = (*Module)(nil)
