package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	spoolresolverv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/spoolresolver/v1"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if info.MinCoreVersion == "" {
		t.Error("MinCoreVersion must not be empty")
	}
	if len(info.Capabilities) == 0 || info.Capabilities[0] != "spool.resolver" {
		t.Errorf("expected spool.resolver capability, got %v", info.Capabilities)
	}
}

func TestResolveTagSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/default" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name":        "default",
			"description": "default tag",
			"version":     "1.0.0",
			"modules": []map[string]any{
				{"repo": "github.com/org/mod", "version": "v1.0.0", "required": true},
			},
		})
	}))
	defer srv.Close()

	m := NewModule(Config{})
	ctx := context.Background()
	resp, err := m.ResolveTag(ctx, &spoolresolverv1.ResolveTagRequest{
		SpoolUrl: srv.URL,
		TagName:  "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.TagDefinitionJson) == 0 {
		t.Fatal("expected non-empty tag definition")
	}

	var tag struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(resp.TagDefinitionJson, &tag); err != nil {
		t.Fatal(err)
	}
	if tag.Name != "default" {
		t.Errorf("expected name=default, got %s", tag.Name)
	}
	if tag.Version != "1.0.0" {
		t.Errorf("expected version=1.0.0, got %s", tag.Version)
	}
}

func TestResolveTagNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := NewModule(Config{})
	ctx := context.Background()
	resp, err := m.ResolveTag(ctx, &spoolresolverv1.ResolveTagRequest{
		SpoolUrl: srv.URL,
		TagName:  "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" {
		t.Fatal("expected error for 404")
	}
}

func TestResolveTagServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewModule(Config{})
	ctx := context.Background()
	resp, err := m.ResolveTag(ctx, &spoolresolverv1.ResolveTagRequest{
		SpoolUrl: srv.URL,
		TagName:  "error",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" {
		t.Fatal("expected error for 500")
	}
}

func TestResolveTagInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	m := NewModule(Config{})
	ctx := context.Background()
	resp, err := m.ResolveTag(ctx, &spoolresolverv1.ResolveTagRequest{
		SpoolUrl: srv.URL,
		TagName:  "bad",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" {
		t.Fatal("expected error for invalid JSON")
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
	if err := m.Health(ctx); err != nil {
		t.Fatal("expected health to pass")
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
