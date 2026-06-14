package internal

import (
	"context"
	"encoding/json"
	"testing"

	dataredactionv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/dataredaction/v1"
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
	if len(info.Contracts) == 0 {
		t.Error("Contracts must not be empty")
	}
	if info.Contracts[0].Interface != "DataRedactionProvider" {
		t.Errorf("expected DataRedactionProvider contract, got %s", info.Contracts[0].Interface)
	}
	if len(info.Capabilities) == 0 {
		t.Error("Capabilities must not be empty")
	}
	if info.Capabilities[0] != "data.redaction" {
		t.Errorf("expected data.redaction capability, got %s", info.Capabilities[0])
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDefaultRedactsSensitiveKeys(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, map[string]any{
		"password": "my-secret",
		"username": "alice",
		"api_key":  "sk-1234",
		"token":    "eyJhbGci",
	})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{Data: input})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	if result["password"] != redactedString {
		t.Errorf("expected password redacted, got %v", result["password"])
	}
	if result["api_key"] != redactedString {
		t.Errorf("expected api_key redacted, got %v", result["api_key"])
	}
	if result["token"] != redactedString {
		t.Errorf("expected token redacted, got %v", result["token"])
	}
	if result["username"] != "alice" {
		t.Errorf("expected username=alice, got %v", result["username"])
	}
}

func TestDefaultRedactsNestedSensitiveKeys(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, map[string]any{
		"user": map[string]any{
			"name":     "alice",
			"password": "secret123",
			"email":    "alice@example.com",
		},
	})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{Data: input})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	user := result["user"].(map[string]any)
	if user["password"] != redactedString {
		t.Errorf("expected nested password redacted, got %v", user["password"])
	}
	if user["name"] != "alice" {
		t.Errorf("expected nested name=alice, got %v", user["name"])
	}
}

func TestFieldRule(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, map[string]any{
		"email":      "alice@example.com",
		"user_email": "bob@example.com",
		"name":       "alice",
	})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{
		Data:  input,
		Rules: []string{"field:email"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	if result["email"] != redactedString {
		t.Errorf("expected email redacted, got %v", result["email"])
	}
	if result["user_email"] != redactedString {
		t.Errorf("expected user_email redacted, got %v", result["user_email"])
	}
	if result["name"] != "alice" {
		t.Errorf("expected name=alice, got %v", result["name"])
	}
}

func TestPathRule(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, map[string]any{
		"user": map[string]any{
			"address": map[string]any{
				"street": "123 Main St",
				"zip":    "90210",
			},
		},
		"other": "data",
	})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{
		Data:  input,
		Rules: []string{"path:user.address.street"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	if result["other"] != "data" {
		t.Errorf("expected other=data, got %v", result["other"])
	}

	addr := result["user"].(map[string]any)["address"].(map[string]any)
	if addr["street"] != redactedString {
		t.Errorf("expected street redacted, got %v", addr["street"])
	}
	if addr["zip"] != "90210" {
		t.Errorf("expected zip=90210, got %v", addr["zip"])
	}
}

func TestRegexRule(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, map[string]any{
		"email": "alice@example.com",
		"phone": "555-123-4567",
		"name":  "alice",
	})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{
		Data:  input,
		Rules: []string{"/\\d{3}-\\d{3}-\\d{4}/"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	if result["phone"] != redactedString {
		t.Errorf("expected phone redacted, got %v", result["phone"])
	}
	if result["email"] != "alice@example.com" {
		t.Errorf("expected email unchanged, got %v", result["email"])
	}
	if result["name"] != "alice" {
		t.Errorf("expected name=alice, got %v", result["name"])
	}
}

func TestNonMapValuePreserved(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, []string{"a", "b", "c"})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{Data: input})
	if err == nil {
		t.Fatalf("expected error for non-object input, got %s", resp.Data)
	}
}

func TestArrayRedact(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	inputObj := map[string]any{"items": []any{
		map[string]any{"name": "alice", "password": "secret1"},
		map[string]any{"name": "bob", "password": "secret2"},
	}}
	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{Data: mustJSON(t, inputObj)})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	items := result["items"].([]any)
	for i, item := range items {
		obj := item.(map[string]any)
		if obj["password"] != redactedString {
			t.Errorf("item %d: expected password redacted, got %v", i, obj["password"])
		}
		if obj["name"] == redactedString {
			t.Errorf("item %d: name should not be redacted", i)
		}
	}
}

func TestSupportedRules(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()
	resp, err := m.SupportedRules(ctx, &dataredactionv1.SupportedRulesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.RuleTypes) == 0 {
		t.Fatal("expected at least one rule type description")
	}
}

func TestPathRuleWithNestedArray(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, map[string]any{
		"users": []any{
			map[string]any{"name": "alice", "ssn": "123-45-6789"},
		},
	})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{
		Data:  input,
		Rules: []string{"/\\d{3}-\\d{2}-\\d{4}/"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	users := result["users"].([]any)
	ssn := users[0].(map[string]any)["ssn"].(string)
	if ssn != redactedString {
		t.Errorf("expected ssn redacted in nested array, got %v", ssn)
	}
}

func TestMultipleRules(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := mustJSON(t, map[string]any{
		"credit_card": "4111-1111-1111-1111",
		"email":       "alice@example.com",
		"name":        "alice",
	})

	resp, err := m.Redact(ctx, &dataredactionv1.RedactRequest{
		Data:  input,
		Rules: []string{"field:credit_card", "/@/"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp.Data, &result)

	if result["credit_card"] != redactedString {
		t.Errorf("expected credit_card redacted by field rule, got %v", result["credit_card"])
	}
	if result["email"] != redactedString {
		t.Errorf("expected email redacted by regex rule, got %v", result["email"])
	}
	if result["name"] != "alice" {
		t.Errorf("expected name=alice, got %v", result["name"])
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
		t.Fatal("expected health to pass after start")
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHealth(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()
	if err := m.Health(ctx); err != nil {
		t.Fatal("expected health to pass")
	}
}
