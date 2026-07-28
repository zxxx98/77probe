package alerting

import (
	"testing"
	"time"
)

func TestEvaluateTransitionsAndRecovery(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	pending := Evaluate(EvaluationInput{State: State{Status: StatusNormal}, Breached: true, Duration: 5 * time.Minute, Now: now})
	if pending.State.Status != StatusPending || pending.Notify {
		t.Fatalf("pending=%+v", pending)
	}
	firing := Evaluate(EvaluationInput{State: pending.State, Breached: true, Duration: 5 * time.Minute, Now: now.Add(5 * time.Minute)})
	if firing.State.Status != StatusFiring || !firing.Notify {
		t.Fatalf("firing=%+v", firing)
	}
	recovered := Evaluate(EvaluationInput{State: firing.State, Breached: false, Duration: 5 * time.Minute, Now: now.Add(6 * time.Minute)})
	if recovered.State.Status != StatusRecovered || !recovered.Notify {
		t.Fatalf("recovered=%+v", recovered)
	}
	normal := Evaluate(EvaluationInput{State: recovered.State, Breached: false, Duration: 5 * time.Minute, Now: now.Add(7 * time.Minute)})
	if normal.State.Status != StatusNormal || normal.Notify {
		t.Fatalf("normal=%+v", normal)
	}
}

func TestEvaluateZeroDurationAndRepeat(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	firing := Evaluate(EvaluationInput{State: State{Status: StatusNormal}, Breached: true, Duration: 0, Now: now})
	if firing.State.Status != StatusFiring || !firing.Notify {
		t.Fatalf("firing=%+v", firing)
	}
	quiet := Evaluate(EvaluationInput{State: firing.State, Breached: true, RepeatInterval: time.Hour, Now: now.Add(30 * time.Minute)})
	if quiet.Notify {
		t.Fatalf("quiet repeat=%+v", quiet)
	}
	repeat := Evaluate(EvaluationInput{State: firing.State, Breached: true, RepeatInterval: time.Hour, Now: now.Add(time.Hour)})
	if !repeat.Notify {
		t.Fatalf("repeat=%+v", repeat)
	}
}
