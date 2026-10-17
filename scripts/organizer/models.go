package main

import "encoding/json"

// RecurrenceSeries is the durable definition shared by recurring tasks and
// events. RuleJSON is versioned so clients do not have to depend on a free
// form RRULE string.
type RecurrenceSeries struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	ItemID      string          `json:"itemId"`
	Kind        string          `json:"kind"`
	RuleVersion int             `json:"ruleVersion"`
	Rule        json.RawMessage `json:"rule"`
	Timezone    string          `json:"timezone"`
	StartsAt    string          `json:"startsAt"`
	EndsAt      string          `json:"endsAt,omitempty"`
	Status      string          `json:"status"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
}

type RecurrenceOccurrence struct {
	ID             string          `json:"id"`
	SeriesID       string          `json:"seriesId"`
	OccurrenceKey  string          `json:"occurrenceKey"`
	ScheduledStart string          `json:"scheduledStart"`
	ScheduledEnd   string          `json:"scheduledEnd,omitempty"`
	Status         string          `json:"status"`
	Override       json.RawMessage `json:"override,omitempty"`
	CompletedAt    string          `json:"completedAt,omitempty"`
	Version        int             `json:"version"`
}

type RecurrenceException struct {
	ID            string          `json:"id"`
	SeriesID      string          `json:"seriesId"`
	OccurrenceKey string          `json:"occurrenceKey"`
	Action        string          `json:"action"`
	Override      json.RawMessage `json:"override,omitempty"`
	CreatedAt     string          `json:"createdAt"`
}

type Capture struct {
	ID             string              `json:"id"`
	WorkspaceID    string              `json:"workspaceId"`
	SourceType     string              `json:"sourceType"`
	RawText        string              `json:"rawText"`
	AttachmentName string              `json:"attachmentName,omitempty"`
	AttachmentMime string              `json:"attachmentMime,omitempty"`
	AttachmentPath string              `json:"-"`
	Attachments    []CaptureAttachment `json:"attachments,omitempty"`
	HasAttachment  bool                `json:"hasAttachment"`
	Status         string              `json:"status"`
	AIResult       json.RawMessage     `json:"aiResult,omitempty"`
	Error          string              `json:"error,omitempty"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
}

type CaptureAttachment struct {
	Name string `json:"name"`
	Mime string `json:"mime"`
	Path string `json:"-"`
}

type Candidate struct {
	Type            string   `json:"type"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	StartAt         string   `json:"startAt,omitempty"`
	EndAt           string   `json:"endAt,omitempty"`
	DueAt           string   `json:"dueAt,omitempty"`
	ReminderAt      string   `json:"reminderAt,omitempty"`
	Timezone        string   `json:"timezone,omitempty"`
	AllDay          bool     `json:"allDay"`
	RecurrenceRule  string   `json:"recurrenceRule,omitempty"`
	Priority        int      `json:"priority"`
	Certainty       string   `json:"certainty,omitempty"`
	DurationMinutes int      `json:"durationMinutes,omitempty"`
	AvailableFrom   string   `json:"availableFrom,omitempty"`
	AvailableUntil  string   `json:"availableUntil,omitempty"`
	Project         string   `json:"project,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Location        string   `json:"location,omitempty"`
	People          []string `json:"people,omitempty"`
	Confidence      float64  `json:"confidence"`
	Ambiguities     []string `json:"ambiguities,omitempty"`
}

type ParseResult struct {
	Items []Candidate `json:"items"`
}

type Item struct {
	ID                 string   `json:"id"`
	WorkspaceID        string   `json:"workspaceId"`
	CaptureID          string   `json:"captureId,omitempty"`
	Type               string   `json:"type"`
	Title              string   `json:"title"`
	Description        string   `json:"description,omitempty"`
	StartAt            string   `json:"startAt,omitempty"`
	EndAt              string   `json:"endAt,omitempty"`
	DueAt              string   `json:"dueAt,omitempty"`
	ReminderAt         string   `json:"reminderAt,omitempty"`
	Timezone           string   `json:"timezone"`
	AllDay             bool     `json:"allDay"`
	RecurrenceRule     string   `json:"recurrenceRule,omitempty"`
	Priority           int      `json:"priority"`
	Status             string   `json:"status"`
	Certainty          string   `json:"certainty"`
	DurationMinutes    int      `json:"durationMinutes,omitempty"`
	AvailableFrom      string   `json:"availableFrom,omitempty"`
	AvailableUntil     string   `json:"availableUntil,omitempty"`
	ProjectID          string   `json:"projectId,omitempty"`
	Project            string   `json:"project,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	Location           string   `json:"location,omitempty"`
	People             []string `json:"people,omitempty"`
	Version            int      `json:"version"`
	CompletedAt        string   `json:"completedAt,omitempty"`
	CancelledAt        string   `json:"cancelledAt,omitempty"`
	ArchivedAt         string   `json:"archivedAt,omitempty"`
	ArchivedFromStatus string   `json:"archivedFromStatus,omitempty"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}

type Project struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspaceId"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	Status             string `json:"status"`
	Priority           int    `json:"priority"`
	Color              string `json:"color,omitempty"`
	Goal               string `json:"goal,omitempty"`
	Deadline           string `json:"deadline,omitempty"`
	ReviewAt           string `json:"reviewAt,omitempty"`
	ArchivedAt         string `json:"archivedAt,omitempty"`
	ArchivedFromStatus string `json:"archivedFromStatus,omitempty"`
	Version            int    `json:"version"`
	ItemCount          int    `json:"itemCount"`
	OpenCount          int    `json:"openCount"`
	DoneCount          int    `json:"doneCount"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

type ProjectEvent struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"projectId"`
	EventType string          `json:"eventType"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt string          `json:"createdAt"`
}

type ItemEvent struct {
	ID        string          `json:"id"`
	ItemID    string          `json:"itemId"`
	EventType string          `json:"eventType"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt string          `json:"createdAt"`
}

type Memory struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspaceId"`
	Kind          string `json:"kind"`
	Key           string `json:"key,omitempty"`
	Content       string `json:"content"`
	Scope         string `json:"scope"`
	Status        string `json:"status"`
	EvidenceCount int    `json:"evidenceCount"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type CorrectionEvent struct {
	ID        string          `json:"id"`
	CaptureID string          `json:"captureId"`
	Field     string          `json:"field"`
	Context   string          `json:"context"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	CreatedAt string          `json:"createdAt"`
}

type PushSubscription struct {
	Endpoint  string `json:"endpoint"`
	P256DH    string `json:"p256dh"`
	Auth      string `json:"auth"`
	UserAgent string `json:"userAgent,omitempty"`
}

type ReminderDelivery struct {
	ReminderID string
	ItemID     string
	Title      string
	RemindAt   string
}

type AssistantConversation struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type AssistantTurn struct {
	ID                  string          `json:"id"`
	WorkspaceID         string          `json:"workspaceId"`
	ConversationID      string          `json:"conversationId"`
	Role                string          `json:"role"`
	Content             string          `json:"content"`
	StructuredResult    json.RawMessage `json:"structuredResult,omitempty"`
	StructuredResultRef string          `json:"structuredResultRef,omitempty"`
	Provider            string          `json:"provider,omitempty"`
	Model               string          `json:"model,omitempty"`
	CreatedAt           string          `json:"createdAt"`
}

// AssistantAction is one closed, validated operation proposed by the assistant.
// Payload is decoded into the concrete payload type selected by Operation.
type AssistantAction struct {
	ActionID               string                 `json:"actionId"`
	Operation              AssistantOperation     `json:"operation"`
	ExpectedItemVersion    *int                   `json:"expectedItemVersion,omitempty"`
	ExpectedItemVersions   map[string]int         `json:"expectedItemVersions,omitempty"`
	ExpectedProjectVersion *int                   `json:"expectedProjectVersion,omitempty"`
	Payload                AssistantActionPayload `json:"payload"`
}

type AssistantPlan struct {
	Kind                 string             `json:"kind,omitempty"`
	Reply                string             `json:"reply,omitempty"`
	IntentSummary        string             `json:"intentSummary"`
	AmbiguityQuestions   []string           `json:"ambiguityQuestions"`
	RiskLevel            AssistantRiskLevel `json:"riskLevel"`
	RequiresConfirmation bool               `json:"requiresConfirmation"`
	Actions              []AssistantAction  `json:"actions"`
}

// AssistantPlanState is the durable lifecycle of an assistant plan. Plans are
// retained as an audit record even when they are cancelled or become stale.
type AssistantPlanState string

const (
	AssistantPlanProposed             AssistantPlanState = "proposed"
	AssistantPlanAwaitingConfirmation AssistantPlanState = "awaiting_confirmation"
	AssistantPlanExecuting            AssistantPlanState = "executing"
	AssistantPlanCompleted            AssistantPlanState = "completed"
	AssistantPlanPartiallyFailed      AssistantPlanState = "partially_failed"
	AssistantPlanCancelled            AssistantPlanState = "cancelled"
	AssistantPlanStale                AssistantPlanState = "stale"
)

type AssistantActionState string

const (
	AssistantActionPending   AssistantActionState = "pending"
	AssistantActionExecuting AssistantActionState = "executing"
	AssistantActionSucceeded AssistantActionState = "succeeded"
	AssistantActionFailed    AssistantActionState = "failed"
	AssistantActionConflict  AssistantActionState = "conflict"
	AssistantActionCancelled AssistantActionState = "cancelled"
)

// AssistantActionResult is retained independently from the plan JSON so a
// receipt can truthfully describe a partial execution and its audit trail.
type AssistantActionResult struct {
	ActionID       string               `json:"actionId"`
	Operation      AssistantOperation   `json:"operation"`
	State          AssistantActionState `json:"state"`
	Error          string               `json:"error,omitempty"`
	BeforeSnapshot json.RawMessage      `json:"beforeSnapshot,omitempty"`
	AfterSnapshot  json.RawMessage      `json:"afterSnapshot,omitempty"`
	Result         json.RawMessage      `json:"result,omitempty"`
	EventIDs       []string             `json:"eventIds,omitempty"`
	Actor          string               `json:"actor"`
	ConversationID string               `json:"conversationId,omitempty"`
	TurnID         string               `json:"turnId,omitempty"`
	StartedAt      string               `json:"startedAt,omitempty"`
	CompletedAt    string               `json:"completedAt,omitempty"`
}

// AssistantPlanRecord is the persisted preview and execution receipt.
type AssistantPlanRecord struct {
	ID                    string                  `json:"id"`
	WorkspaceID           string                  `json:"workspaceId"`
	ConversationID        string                  `json:"conversationId,omitempty"`
	TurnID                string                  `json:"turnId,omitempty"`
	IdempotencyKey        string                  `json:"idempotencyKey"`
	RequestHash           string                  `json:"requestHash"`
	SourceRequestHash     string                  `json:"-"`
	State                 AssistantPlanState      `json:"state"`
	Plan                  AssistantPlan           `json:"plan"`
	RequiresConfirmation  bool                    `json:"requiresConfirmation"`
	ConfirmationExpiresAt string                  `json:"confirmationExpiresAt,omitempty"`
	Actor                 string                  `json:"actor"`
	Actions               []AssistantActionResult `json:"actions"`
	Error                 string                  `json:"error,omitempty"`
	CreatedAt             string                  `json:"createdAt"`
	UpdatedAt             string                  `json:"updatedAt"`
	StartedAt             string                  `json:"startedAt,omitempty"`
	CompletedAt           string                  `json:"completedAt,omitempty"`
	LeaseUntil            string                  `json:"leaseUntil,omitempty"`
}

// AssistantPlanRequest is used to create a durable preview. IdempotencyKey is
// scoped to WorkspaceID and must be stable across network retries.
type AssistantPlanRequest struct {
	WorkspaceID       string        `json:"workspaceId"`
	ConversationID    string        `json:"conversationId,omitempty"`
	TurnID            string        `json:"turnId,omitempty"`
	IdempotencyKey    string        `json:"idempotencyKey"`
	SourceRequestHash string        `json:"-"`
	Actor             string        `json:"actor,omitempty"`
	Plan              AssistantPlan `json:"plan"`
}

type AssistantExecuteRequest struct {
	WorkspaceID    string `json:"workspaceId"`
	PlanID         string `json:"planId"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	Actor          string `json:"actor,omitempty"`
	Confirmed      bool   `json:"confirmed"`
}
