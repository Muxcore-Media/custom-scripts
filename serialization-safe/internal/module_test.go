package internal

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
	serializationv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/serialization/v1"
	"github.com/vmihailenco/msgpack/v5"
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
	if info.Contracts[0].Interface != "SerializationProvider" {
		t.Errorf("expected SerializationProvider contract, got %s", info.Contracts[0].Interface)
	}
	if len(info.Capabilities) == 0 {
		t.Error("Capabilities must not be empty")
	}
	if info.Capabilities[0] != "serialization" {
		t.Errorf("expected serialization capability, got %s", info.Capabilities[0])
	}
}

func TestSupportedTypes(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()
	resp, err := m.SupportedTypes(ctx, &serializationv1.SupportedTypesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{contracts.SafeContentTypeJSON, contracts.SafeContentTypeMsgpack}
	if len(resp.ContentTypes) != len(expected) {
		t.Fatalf("expected %d types, got %d: %v", len(expected), len(resp.ContentTypes), resp.ContentTypes)
	}
	for i, ct := range resp.ContentTypes {
		if ct != expected[i] {
			t.Errorf("type %d: expected %s, got %s", i, expected[i], ct)
		}
	}
}

func TestJSONToMsgpack(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := []byte(`{"name":"test","count":42,"active":true,"tags":["a","b"]}`)
	resp, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeJSON,
		TargetContentType: contracts.SafeContentTypeMsgpack,
		Data:              input,
	})
	if err != nil {
		t.Fatal(err)
	}

	var v any
	if err := msgpack.Unmarshal(resp.Result, &v); err != nil {
		t.Fatalf("msgpack decode result: %v", err)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", v)
	}
	if obj["name"] != "test" {
		t.Errorf("expected name=test, got %v", obj["name"])
	}
	if obj["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", obj["count"])
	}
	if obj["active"] != true {
		t.Errorf("expected active=true, got %v", obj["active"])
	}
}

func TestMsgpackToJSON(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	original := map[string]any{"name": "test", "value": 3.14}
	mp, err := msgpack.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeMsgpack,
		TargetContentType: contracts.SafeContentTypeJSON,
		Data:              mp,
	})
	if err != nil {
		t.Fatal(err)
	}

	var v map[string]any
	if err := json.Unmarshal(resp.Result, &v); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if v["name"] != "test" {
		t.Errorf("expected name=test, got %v", v["name"])
	}
	if v["value"] != 3.14 {
		t.Errorf("expected value=3.14, got %v", v["value"])
	}
}

func TestRoundTrip(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	original := []byte(`{"a":1,"b":"two","c":[1,2,3]}`)

	toMsgpack, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeJSON,
		TargetContentType: contracts.SafeContentTypeMsgpack,
		Data:              original,
	})
	if err != nil {
		t.Fatal(err)
	}

	backToJSON, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeMsgpack,
		TargetContentType: contracts.SafeContentTypeJSON,
		Data:              toMsgpack.Result,
	})
	if err != nil {
		t.Fatal(err)
	}

	var expected, got map[string]any
	if err := json.Unmarshal(original, &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(backToJSON.Result, &got); err != nil {
		t.Fatal(err)
	}
	if expected["a"] != got["a"] {
		t.Errorf("a: expected %v, got %v", expected["a"], got["a"])
	}
	if expected["b"] != got["b"] {
		t.Errorf("b: expected %v, got %v", expected["b"], got["b"])
	}
}

func TestPassthrough(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := []byte(`{"unchanged":true}`)
	resp, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeJSON,
		TargetContentType: contracts.SafeContentTypeJSON,
		Data:              input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Result) != string(input) {
		t.Errorf("passthrough: expected %s, got %s", input, resp.Result)
	}
}

func TestUnsupportedSource(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	_, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: "application/yaml",
		TargetContentType: contracts.SafeContentTypeJSON,
		Data:              []byte("key: value\n"),
	})
	if err == nil {
		t.Fatal("expected error for unsupported source type")
	}
}

func TestUnsupportedTarget(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	_, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeJSON,
		TargetContentType: "application/gob",
		Data:              []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected error for unsupported target type")
	}
}

func TestJSONArray(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	input := []byte(`[1,"two",{"three":3}]`)
	resp, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeJSON,
		TargetContentType: contracts.SafeContentTypeMsgpack,
		Data:              input,
	})
	if err != nil {
		t.Fatal(err)
	}

	var v []any
	if err := msgpack.Unmarshal(resp.Result, &v); err != nil {
		t.Fatalf("msgpack decode: %v", err)
	}
	if len(v) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(v))
	}
}

func TestInvalidJSON(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	_, err := m.Convert(ctx, &serializationv1.ConvertRequest{
		SourceContentType: contracts.SafeContentTypeJSON,
		TargetContentType: contracts.SafeContentTypeMsgpack,
		Data:              []byte(`{invalid}`),
	})
	if err == nil {
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
