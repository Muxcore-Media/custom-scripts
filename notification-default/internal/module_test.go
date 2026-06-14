package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	notifyv1 "github.com/Muxcore-Media/notification-default/proto/notifyv1"
)

func newTestModule(t *testing.T) *Module {
	t.Helper()
	m := NewModule(Config{
		GRPCAddr:       ":0",
		DiscordWebhook: "https://discord.example.com/webhook",
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { m.Stop(ctx) })
	return m
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
	if len(info.Capabilities) == 0 || info.Capabilities[0] != "notification" {
		t.Errorf("expected notification capability, got %v", info.Capabilities)
	}
	if len(info.Roles) == 0 || info.Roles[0] != "notification" {
		t.Errorf("expected role notification, got %v", info.Roles)
	}
}

func TestNotifyDiscord(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", DiscordWebhook: srv.URL})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)

	resp, err := m.Notify(ctx, &notifyv1.NotifyRequest{
		Title:        "Test",
		Message:      "Hello Discord",
		Severity:     "info",
		SourceModule: "test",
		Channels:     []notifyv1.Channel{notifyv1.Channel_CHANNEL_DISCORD},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if !resp.Results[0].Success {
		t.Fatalf("discord webhook failed: %s", resp.Results[0].Error)
	}

	embeds, ok := received["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatal("expected discord embed")
	}
	embed := embeds[0].(map[string]any)
	if embed["title"] != "Test" {
		t.Errorf("expected title 'Test', got %v", embed["title"])
	}
}

func TestNotifySlack(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", SlackWebhook: srv.URL})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)

	resp, err := m.Notify(ctx, &notifyv1.NotifyRequest{
		Title:    "Test Slack",
		Message:  "Hello Slack",
		Severity: "warning",
		Channels: []notifyv1.Channel{notifyv1.Channel_CHANNEL_SLACK},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Results[0].Success {
		t.Fatalf("slack webhook failed: %s", resp.Results[0].Error)
	}
}

func TestNotifyGenericWebhook(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", GenericWebhook: srv.URL})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)

	resp, err := m.Notify(ctx, &notifyv1.NotifyRequest{
		Title:   "Generic",
		Message: "Hello Webhook",
		Fields:  map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Results[0].Success {
		t.Fatalf("webhook failed: %s", resp.Results[0].Error)
	}
}

func TestNotifyWebhookFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", DiscordWebhook: srv.URL})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)

	resp, err := m.Notify(ctx, &notifyv1.NotifyRequest{
		Title:    "Fail",
		Message:  "Should fail",
		Channels: []notifyv1.Channel{notifyv1.Channel_CHANNEL_DISCORD},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Results[0].Success {
		t.Fatal("expected webhook failure")
	}
}

func TestUnconfiguredChannel(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	resp, err := m.Notify(ctx, &notifyv1.NotifyRequest{
		Title:    "Test",
		Message:  "No Slack configured",
		Channels: []notifyv1.Channel{notifyv1.Channel_CHANNEL_SLACK},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Results[0].Success {
		t.Fatal("expected failure for unconfigured channel")
	}
}

func TestNotifyAllChannels(t *testing.T) {
	var discordCalled, slackCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{
		GRPCAddr:       ":0",
		DiscordWebhook: srv.URL,
		SlackWebhook:   srv.URL + "/slack",
	})
	m.client = srv.Client()
	_ = discordCalled
	_ = slackCalled

	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)

	resp, err := m.Notify(ctx, &notifyv1.NotifyRequest{
		Title:   "All channels",
		Message: "Test all",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
}

func TestConfigure(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)

	resp, err := m.Configure(ctx, &notifyv1.ConfigureRequest{
		Channel:  notifyv1.Channel_CHANNEL_DISCORD,
		Settings: map[string]string{"webhook_url": "https://discord.example.com/new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Configured {
		t.Fatal("expected configured=true")
	}
}

func TestStatus(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0", DiscordWebhook: "https://discord.gg/webhook", SlackWebhook: "https://slack.com/webhook"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)

	resp, err := m.Status(ctx, &notifyv1.StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(resp.Channels))
	}
}

func TestHealthNoChannels(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0"})
	ctx := context.Background()
	if err := m.Health(ctx); err == nil {
		t.Fatal("expected health error with no channels configured")
	}
}

func TestHealthWithChannel(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0", DiscordWebhook: "https://discord.gg/webhook"})
	ctx := context.Background()
	if err := m.Health(ctx); err != nil {
		t.Fatalf("expected health ok with channel, got %v", err)
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
