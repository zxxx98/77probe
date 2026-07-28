package alerting

import "time"

type EvaluationInput struct {
	State          State
	Breached       bool
	CurrentValue   float64
	Duration       time.Duration
	RepeatInterval time.Duration
	Now            time.Time
}

type EvaluationResult struct {
	State  State
	Notify bool
}

func Evaluate(input EvaluationInput) EvaluationResult {
	now := input.Now.UTC()
	state := input.State
	if !validStatus(state.Status) {
		state.Status = StatusNormal
	}
	state.LastValue = floatPointer(input.CurrentValue)

	switch state.Status {
	case StatusNormal:
		if !input.Breached {
			return finishEvaluation(state, now, false)
		}
		if input.Duration <= 0 {
			state.Status = StatusFiring
			state.PendingSince = nil
			state.FiringSince = timePointer(now)
			state.LastNotifiedAt = timePointer(now)
			return finishEvaluation(state, now, true)
		}
		state.Status = StatusPending
		state.PendingSince = timePointer(now)
		state.FiringSince = nil
		return finishEvaluation(state, now, false)
	case StatusPending:
		if !input.Breached {
			state.Status = StatusNormal
			state.PendingSince = nil
			state.FiringSince = nil
			return finishEvaluation(state, now, false)
		}
		if state.PendingSince == nil {
			state.PendingSince = timePointer(now)
			return finishEvaluation(state, now, false)
		}
		if now.Sub(*state.PendingSince) < input.Duration {
			return finishEvaluation(state, now, false)
		}
		state.Status = StatusFiring
		state.FiringSince = timePointer(now)
		state.LastNotifiedAt = timePointer(now)
		return finishEvaluation(state, now, true)
	case StatusFiring:
		if !input.Breached {
			state.Status = StatusRecovered
			state.PendingSince = nil
			state.LastNotifiedAt = timePointer(now)
			return finishEvaluation(state, now, true)
		}
		if state.FiringSince == nil {
			state.FiringSince = timePointer(now)
		}
		if input.RepeatInterval > 0 && (state.LastNotifiedAt == nil || now.Sub(*state.LastNotifiedAt) >= input.RepeatInterval) {
			state.LastNotifiedAt = timePointer(now)
			return finishEvaluation(state, now, true)
		}
		return finishEvaluation(state, now, false)
	case StatusRecovered:
		if !input.Breached {
			state.Status = StatusNormal
			state.PendingSince = nil
			state.FiringSince = nil
			return finishEvaluation(state, now, false)
		}
		state.FiringSince = nil
		state.LastNotifiedAt = nil
		if input.Duration <= 0 {
			state.Status = StatusFiring
			state.PendingSince = nil
			state.FiringSince = timePointer(now)
			state.LastNotifiedAt = timePointer(now)
			return finishEvaluation(state, now, true)
		}
		state.Status = StatusPending
		state.PendingSince = timePointer(now)
		return finishEvaluation(state, now, false)
	default:
		state.Status = StatusNormal
		return finishEvaluation(state, now, false)
	}
}

func finishEvaluation(state State, now time.Time, notify bool) EvaluationResult {
	state.UpdatedAt = now
	return EvaluationResult{State: state, Notify: notify}
}

func timePointer(value time.Time) *time.Time { return &value }
func floatPointer(value float64) *float64    { return &value }
