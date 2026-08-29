package workflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/koto7/tuma/internal/crypto"
	"github.com/koto7/tuma/internal/metrics"
	"github.com/koto7/tuma/internal/storage"
)

const (
	TaskQueue       = "tuma-delivery"
	WorkflowVersion = 1
)

type Input struct {
	EventID uuid.UUID
}

type Activities struct {
	Store      *storage.Store
	Encryptor  *crypto.Encryptor
	HTTPClient *http.Client
	Semaphores *ConnectionSemaphores
}

type DeliverResult struct {
	Success      bool
	ResponseCode int
	LatencyMS    int
	Error        string
	DeliveryID   uuid.UUID
}

type ConnectionSemaphores struct {
	mu    sync.Mutex
	limit int
	pools map[uuid.UUID]chan struct{}
}

func NewConnectionSemaphores(limit int) *ConnectionSemaphores {
	return &ConnectionSemaphores{limit: limit, pools: make(map[uuid.UUID]chan struct{})}
}

func (cs *ConnectionSemaphores) acquire(connID uuid.UUID) func() {
	cs.mu.Lock()
	pool, ok := cs.pools[connID]
	if !ok {
		pool = make(chan struct{}, cs.limit)
		cs.pools[connID] = pool
	}
	cs.mu.Unlock()
	pool <- struct{}{}
	return func() { <-pool }
}

func DeliveryWorkflow(ctx workflow.Context, eventID uuid.UUID) error {
	v := workflow.GetVersion(ctx, "initial", workflow.DefaultVersion, WorkflowVersion)

	var acts Activities

	actOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1},
	}
	ctx = workflow.WithActivityOptions(ctx, actOpts)

	var meta struct {
		RetryAttempts      int
		RetryFirstDelayS   int
		RetryBackoffFactor float64
	}
	if err := workflow.ExecuteActivity(ctx, acts.LoadEventMeta, eventID).Get(ctx, &meta); err != nil {
		return err
	}

	maxAttempts := meta.RetryAttempts
	if v >= WorkflowVersion {
		// version branch established for future control-flow changes
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var result DeliverResult
		err := workflow.ExecuteActivity(ctx, acts.DeliverActivity, eventID, attempt).Get(ctx, &result)
		if err != nil {
			return err
		}
		if result.Success {
			return workflow.ExecuteActivity(ctx, acts.RecordDelivered, eventID, attempt, result).Get(ctx, nil)
		}
		if attempt < maxAttempts {
			delay := backoff(meta.RetryFirstDelayS, meta.RetryBackoffFactor, attempt)
			_ = workflow.Sleep(ctx, delay)
		}
	}

	return workflow.ExecuteActivity(ctx, acts.RecordIssue, eventID, maxAttempts).Get(ctx, nil)
}

func backoff(firstDelayS int, factor float64, attempt int) time.Duration {
	delay := float64(firstDelayS)
	for i := 1; i < attempt; i++ {
		delay *= factor
	}
	return time.Duration(delay) * time.Second
}

func (a *Activities) LoadEventMeta(ctx context.Context, eventID uuid.UUID) (struct {
	RetryAttempts      int
	RetryFirstDelayS   int
	RetryBackoffFactor float64
}, error) {
	var out struct {
		RetryAttempts      int
		RetryFirstDelayS   int
		RetryBackoffFactor float64
	}
	event, err := a.Store.GetEvent(ctx, eventID)
	if err != nil || event == nil {
		return out, fmt.Errorf("event not found")
	}
	conn, err := a.Store.GetConnection(ctx, event.ConnectionID)
	if err != nil || conn == nil {
		return out, fmt.Errorf("connection not found")
	}
	out.RetryAttempts = conn.RetryAttempts
	out.RetryFirstDelayS = conn.RetryFirstDelayS
	out.RetryBackoffFactor = conn.RetryBackoffFactor
	return out, nil
}

func (a *Activities) DeliverActivity(ctx context.Context, eventID uuid.UUID, attempt int) (DeliverResult, error) {
	logger := activity.GetLogger(ctx)
	event, err := a.Store.GetEvent(ctx, eventID)
	if err != nil || event == nil {
		return DeliverResult{}, fmt.Errorf("event not found")
	}
	conn, err := a.Store.GetConnection(ctx, event.ConnectionID)
	if err != nil || conn == nil {
		return DeliverResult{}, fmt.Errorf("connection not found")
	}

	release := a.Semaphores.acquire(conn.ID)
	defer release()

	deliveryID := uuid.New()
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, conn.DestinationURL, strings.NewReader(string(event.RawPayload)))
	if err != nil {
		return DeliverResult{Success: false, Error: err.Error(), DeliveryID: deliveryID}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tuma-Delivery-Id", deliveryID.String())
	req.Header.Set("X-Tuma-Event-Id", event.ProviderEventID)
	req.Header.Set("X-Tuma-Attempt", fmt.Sprintf("%d", attempt))

	resp, err := a.HTTPClient.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		metrics.ConnectionErrors.WithLabelValues(conn.ID.String()).Inc()
		metrics.DeliveryLatency.WithLabelValues(conn.ID.String(), "failed").Observe(float64(latency))
		_ = a.Store.RecordDelivery(ctx, &storage.Delivery{
			EventID: eventID, AttemptNumber: attempt, Status: "failed",
			LatencyMS: &latency,
		})
		logger.Warn("delivery failed", "error", err)
		return DeliverResult{Success: false, LatencyMS: latency, Error: err.Error(), DeliveryID: deliveryID}, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	code := resp.StatusCode
	success := code >= 200 && code < 300
	status := "failed"
	if success {
		status = "delivered"
	} else {
		metrics.ConnectionErrors.WithLabelValues(conn.ID.String()).Inc()
	}
	metrics.DeliveryLatency.WithLabelValues(conn.ID.String(), status).Observe(float64(latency))

	deliveryStatus := "failed"
	if success {
		deliveryStatus = "delivered"
	}
	_ = a.Store.RecordDelivery(ctx, &storage.Delivery{
		EventID: eventID, AttemptNumber: attempt, Status: deliveryStatus,
		ResponseCode: &code, LatencyMS: &latency,
	})

	if success {
		return DeliverResult{Success: true, ResponseCode: code, LatencyMS: latency, DeliveryID: deliveryID}, nil
	}
	return DeliverResult{
		Success: false, ResponseCode: code, LatencyMS: latency,
		Error: fmt.Sprintf("destination returned %d", code), DeliveryID: deliveryID,
	}, nil
}

func (a *Activities) RecordDelivered(ctx context.Context, eventID uuid.UUID, attempt int, result DeliverResult) error {
	if err := a.Store.ResolveIssueByEventID(ctx, eventID); err != nil {
		return err
	}
	if n, err := a.Store.OpenIssuesCount(ctx); err == nil {
		metrics.SetDLQDepth(float64(n))
	}
	return nil
}

func (a *Activities) RecordIssue(ctx context.Context, eventID uuid.UUID, attempts int) error {
	event, err := a.Store.GetEvent(ctx, eventID)
	if err != nil || event == nil {
		return fmt.Errorf("event not found")
	}
	issue := &storage.Issue{
		EventID:           eventID,
		ConnectionID:      event.ConnectionID,
		Reason:            "Delivery attempts exhausted",
		AttemptsExhausted: attempts,
		FirstFailedAt:     time.Now(),
	}
	if err := a.Store.RecordIssue(ctx, issue); err != nil {
		return err
	}
	if n, err := a.Store.OpenIssuesCount(ctx); err == nil {
		metrics.SetDLQDepth(float64(n))
	}
	return nil
}

func WorkflowID(eventID uuid.UUID) string {
	return fmt.Sprintf("delivery-%s", eventID)
}

func ReplayWorkflowID(eventID uuid.UUID) string {
	return fmt.Sprintf("delivery-replay-%s-%d", eventID, time.Now().UnixNano())
}
