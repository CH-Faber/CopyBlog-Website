package main

import "encoding/json"

type Capture struct {
	ID             string          `json:"id"`
	SourceType     string          `json:"sourceType"`
	RawText        string          `json:"rawText"`
	AttachmentName string          `json:"attachmentName,omitempty"`
	AttachmentMime string          `json:"attachmentMime,omitempty"`
	AttachmentPath string          `json:"-"`
	HasAttachment  bool            `json:"hasAttachment"`
	Status         string          `json:"status"`
	AIResult       json.RawMessage `json:"aiResult,omitempty"`
	Error          string          `json:"error,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
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
	ID              string   `json:"id"`
	CaptureID       string   `json:"captureId,omitempty"`
	Type            string   `json:"type"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	StartAt         string   `json:"startAt,omitempty"`
	EndAt           string   `json:"endAt,omitempty"`
	DueAt           string   `json:"dueAt,omitempty"`
	ReminderAt      string   `json:"reminderAt,omitempty"`
	Timezone        string   `json:"timezone"`
	AllDay          bool     `json:"allDay"`
	RecurrenceRule  string   `json:"recurrenceRule,omitempty"`
	Priority        int      `json:"priority"`
	Status          string   `json:"status"`
	Certainty       string   `json:"certainty"`
	DurationMinutes int      `json:"durationMinutes,omitempty"`
	AvailableFrom   string   `json:"availableFrom,omitempty"`
	AvailableUntil  string   `json:"availableUntil,omitempty"`
	ProjectID       string   `json:"projectId,omitempty"`
	Project         string   `json:"project,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Location        string   `json:"location,omitempty"`
	People          []string `json:"people,omitempty"`
	Version         int      `json:"version"`
	CompletedAt     string   `json:"completedAt,omitempty"`
	CancelledAt     string   `json:"cancelledAt,omitempty"`
	ArchivedAt      string   `json:"archivedAt,omitempty"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	Color       string `json:"color,omitempty"`
	Goal        string `json:"goal,omitempty"`
	Deadline    string `json:"deadline,omitempty"`
	ReviewAt    string `json:"reviewAt,omitempty"`
	ItemCount   int    `json:"itemCount"`
	OpenCount   int    `json:"openCount"`
	DoneCount   int    `json:"doneCount"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
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
