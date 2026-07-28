package alerting

import (
	"context"
	"sync"
	"time"
)

const (
	deliveryQueueCapacity = 64
	deliveryWorkerCount   = 2
)

type DeliveryJob struct {
	Event      Event
	Config     WebhookConfig
	Data       TemplateData
	IsTest     bool
	Completion chan DeliveryOutcome
}

type DeliveryOutcome struct {
	Success  bool      `json:"success"`
	Attempts []Attempt `json:"attempts"`
}

type attemptStore interface {
	RecordAttempt(context.Context, Attempt) (Attempt, error)
}

type Dispatcher struct {
	store     attemptStore
	client    *WebhookClient
	queue     chan DeliveryJob
	sleep     func(context.Context, time.Duration) error
	workers   sync.WaitGroup
	closeOnce sync.Once
}

func NewDispatcher(store attemptStore, client *WebhookClient) *Dispatcher {
	if store == nil || client == nil {
		panic("alerting dispatcher requires store and webhook client")
	}
	return &Dispatcher{
		store:  store,
		client: client,
		queue:  make(chan DeliveryJob, deliveryQueueCapacity),
		sleep:  sleepContext,
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	for range deliveryWorkerCount {
		d.workers.Add(1)
		go func() {
			defer d.workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-d.queue:
					if !ok {
						return
					}
					d.dispatch(ctx, job)
				}
			}
		}()
	}
	<-ctx.Done()
	d.workers.Wait()
}

func (d *Dispatcher) Close() {
	d.closeOnce.Do(func() { close(d.queue) })
}

func (d *Dispatcher) Enqueue(job DeliveryJob) error {
	select {
	case d.queue <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

func (d *Dispatcher) DispatchNow(ctx context.Context, job DeliveryJob) DeliveryOutcome {
	return d.dispatch(ctx, job)
}

func (d *Dispatcher) dispatch(ctx context.Context, job DeliveryJob) DeliveryOutcome {
	body, err := RenderTemplate(job.Config.BodyTemplate, job.Data)
	if err != nil {
		outcome := DeliveryOutcome{Attempts: []Attempt{{Attempt: 1, IsTest: job.IsTest, ErrorText: limitAttemptError(err.Error()), SentAt: time.Now().UTC()}}}
		if job.Event.ID > 0 {
			eventID := job.Event.ID
			outcome.Attempts[0].EventID = &eventID
		}
		persisted, storeErr := d.store.RecordAttempt(ctx, outcome.Attempts[0])
		if storeErr == nil {
			outcome.Attempts[0] = persisted
		}
		d.complete(job, outcome)
		return outcome
	}
	delays := [...]time.Duration{0, 5 * time.Second, 15 * time.Second}
	outcome := DeliveryOutcome{Attempts: make([]Attempt, 0, len(delays))}
	for index, delay := range delays {
		if delay > 0 && d.sleep(ctx, delay) != nil {
			break
		}
		result := d.client.Send(ctx, job.Config, body)
		attempt := Attempt{IsTest: job.IsTest, Attempt: index + 1, ResponseStatus: result.ResponseStatus, ErrorText: result.ErrorText, SentAt: time.Now().UTC()}
		if job.Event.ID > 0 {
			eventID := job.Event.ID
			attempt.EventID = &eventID
		}
		persisted, err := d.store.RecordAttempt(ctx, attempt)
		if err == nil {
			attempt = persisted
		}
		outcome.Attempts = append(outcome.Attempts, attempt)
		if result.Success() {
			outcome.Success = true
			break
		}
	}
	d.complete(job, outcome)
	return outcome
}

func (d *Dispatcher) complete(job DeliveryJob, outcome DeliveryOutcome) {
	if job.Completion == nil {
		return
	}
	select {
	case job.Completion <- outcome:
	default:
	}
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
