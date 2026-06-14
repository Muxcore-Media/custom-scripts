package internal

import (
	"context"
	"testing"

	inputvalidatev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/inputvalidate/v1"
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
	if len(info.Capabilities) == 0 || info.Capabilities[0] != "input.validate" {
		t.Errorf("expected input.validate capability, got %v", info.Capabilities)
	}
}

func TestSupportedSchemas(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()
	resp, err := m.SupportedSchemas(ctx, &inputvalidatev1.SupportedSchemasRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Schemas) == 0 {
		t.Fatal("expected at least one schema type")
	}
}

func TestValidateJSONSchemaInline(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	schema := `{"type": "object", "properties": {"name": {"type": "string"}, "age": {"type": "number"}}, "required": ["name"]}`
	data := []byte(`{"name": "alice", "age": 30}`)

	resp, err := m.Validate(ctx, &inputvalidatev1.ValidateRequest{
		Schema: "json:" + schema,
		Data:   data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Valid {
		t.Fatalf("expected valid, got errors: %v", resp.Errors)
	}
}

func TestValidateJSONSchemaInvalid(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	schema := `{"type": "object", "properties": {"email": {"type": "string", "format": "email"}}, "required": ["email"]}`
	data := []byte(`{"email": "not-an-email"}`)

	resp, err := m.Validate(ctx, &inputvalidatev1.ValidateRequest{
		Schema: "json:" + schema,
		Data:   data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Valid {
		t.Fatal("expected invalid")
	}
	if len(resp.Errors) == 0 {
		t.Fatal("expected validation errors")
	}
}

func TestValidateJSONSchemaMissingField(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	schema := `{"type": "object", "required": ["required_field"]}`
	data := []byte(`{}`)

	resp, err := m.Validate(ctx, &inputvalidatev1.ValidateRequest{
		Schema: "json:" + schema,
		Data:   data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Valid {
		t.Fatal("expected invalid for missing required field")
	}
}

func TestValidateRegexMatch(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	resp, err := m.Validate(ctx, &inputvalidatev1.ValidateRequest{
		Schema: "regex:^[a-z]+$",
		Data:   []byte("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Valid {
		t.Fatalf("expected valid, got errors: %v", resp.Errors)
	}
}

func TestValidateRegexNoMatch(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	resp, err := m.Validate(ctx, &inputvalidatev1.ValidateRequest{
		Schema: "regex:^[a-z]+$",
		Data:   []byte("Hello123"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Valid {
		t.Fatal("expected invalid")
	}
}

func TestValidateUnsupportedSchema(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	resp, err := m.Validate(ctx, &inputvalidatev1.ValidateRequest{
		Schema: "sql:3",
		Data:   []byte("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Valid {
		t.Fatal("expected invalid for unsupported schema type")
	}
}

func TestValidateNotJSON(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	schema := `{"type": "object"}`
	resp, err := m.Validate(ctx, &inputvalidatev1.ValidateRequest{
		Schema: "json:" + schema,
		Data:   []byte("not json at all"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Valid {
		t.Fatal("expected invalid for non-JSON input")
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
