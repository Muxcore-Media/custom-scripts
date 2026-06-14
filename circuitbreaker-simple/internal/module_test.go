package internal

import (
	"context"
	"testing"
	"time"

	circuitbreakerv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/circuitbreaker/v1"
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
	if info.Contracts[0].Interface != "CircuitBreaker" {
		t.Errorf("expected CircuitBreaker contract, got %s", info.Contracts[0].Interface)
	}
	if len(info.Capabilities) == 0 {
		t.Error("Capabilities must not be empty")
	}
	if info.Capabilities[0] != "circuitbreaker" {
		t.Errorf("expected circuitbreaker capability, got %s", info.Capabilities[0])
	}
}

func TestInitialStateClosed(t *testing.T) {
	m := NewModule(Config{})
	ctx := context.Background()

	resp, err := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED {
		t.Errorf("expected closed, got %v", resp.State)
	}
	if resp.FailureCount != 0 {
		t.Errorf("expected 0 failures, got %d", resp.FailureCount)
	}
}

func TestOpensAfterThreshold(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 3})
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		resp, err := m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "fail-key"})
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 && resp.CircuitJustOpened {
			t.Fatalf("circuit opened early at failure %d", i+1)
		}
		if i == 2 && !resp.CircuitJustOpened {
			t.Fatal("expected circuit to open on 3rd failure")
		}
		if i == 2 && resp.NewState != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
			t.Errorf("expected open state, got %v", resp.NewState)
		}
	}

	resp, err := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "fail-key"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
		t.Errorf("expected open, got %v", resp.State)
	}
	if resp.FailureCount != 3 {
		t.Errorf("expected 3 failures, got %d", resp.FailureCount)
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 5})
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "reset-key"})
	}

	m.RecordSuccess(ctx, &circuitbreakerv1.RecordSuccessRequest{Key: "reset-key"})

	resp, err := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "reset-key"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.FailureCount != 0 {
		t.Errorf("expected 0 failures after success, got %d", resp.FailureCount)
	}
}

func TestHalfOpenToClosedOnSuccess(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 2, Cooldown: 1 * time.Millisecond})
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "halfopen-key"})
	}

	resp, err := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "halfopen-key"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
		t.Fatalf("expected open, got %v", resp.State)
	}

	time.Sleep(5 * time.Millisecond)

	success, err := m.RecordSuccess(ctx, &circuitbreakerv1.RecordSuccessRequest{Key: "halfopen-key"})
	if err != nil {
		t.Fatal(err)
	}
	if success.NewState != circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED {
		t.Errorf("expected closed after success in half-open, got %v", success.NewState)
	}
}

func TestHalfOpenToOpenOnFailure(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 2, Cooldown: 1 * time.Millisecond})
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "reopen-key"})
	}

	time.Sleep(5 * time.Millisecond)

	fail, err := m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "reopen-key"})
	if err != nil {
		t.Fatal(err)
	}
	if fail.NewState != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
		t.Errorf("expected open after failure in half-open, got %v", fail.NewState)
	}
	if !fail.CircuitJustOpened {
		t.Error("expected circuit_just_opened=true for half-open->open transition")
	}
}

func TestReset(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 2})
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "reset-key"})
	}

	reset, err := m.Reset(ctx, &circuitbreakerv1.ResetRequest{Key: "reset-key"})
	if err != nil {
		t.Fatal(err)
	}
	if reset.PreviousState != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
		t.Errorf("expected previous state open, got %v", reset.PreviousState)
	}

	resp, err := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "reset-key"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED {
		t.Errorf("expected closed after reset, got %v", resp.State)
	}
	if resp.FailureCount != 0 {
		t.Errorf("expected 0 failures after reset, got %d", resp.FailureCount)
	}
}

func TestMultipleKeysIndependent(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 2})
	ctx := context.Background()

	m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "key-a"})
	m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "key-a"})
	m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "key-b"})

	stateA, _ := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "key-a"})
	stateB, _ := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "key-b"})

	if stateA.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
		t.Errorf("key-a expected open, got %v", stateA.State)
	}
	if stateB.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_CLOSED {
		t.Errorf("key-b expected closed, got %v", stateB.State)
	}
}

func TestStateTimestamps(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 1, Cooldown: 30 * time.Second})
	ctx := context.Background()

	m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "ts-key"})

	resp, err := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "ts-key"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
		t.Errorf("expected open, got %v", resp.State)
	}
	if resp.OpenedAtUnixNano == 0 {
		t.Error("expected opened_at timestamp")
	}
	if resp.HalfOpenAfterUnixNano == 0 {
		t.Error("expected half_open_after timestamp")
	}
	if resp.HalfOpenAfterUnixNano <= resp.OpenedAtUnixNano {
		t.Error("half_open_after should be after opened_at")
	}
}

func TestPreconfiguredThreshold(t *testing.T) {
	m := NewModule(Config{FailureThreshold: 10})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		m.RecordFailure(ctx, &circuitbreakerv1.RecordFailureRequest{Key: "preconfig"})
	}

	resp, err := m.State(ctx, &circuitbreakerv1.StateRequest{Key: "preconfig"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State != circuitbreakerv1.CircuitState_CIRCUIT_STATE_OPEN {
		t.Errorf("expected open after 10 failures, got %v", resp.State)
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
