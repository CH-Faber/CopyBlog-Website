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
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	StartAt        string   `json:"startAt,omitempty"`
	EndAt          string   `json:"endAt,omitempty"`
	DueAt          string   `json:"dueAt,omitempty"`
	ReminderAt     string   `json:"reminderAt,omitempty"`
	Timezone       string   `json:"timezone,omitempty"`
	AllDay         bool     `json:"allDay"`
	RecurrenceRule string   `json:"recurrenceRule,omitempty"`
	Priority       int      `json:"priority"`
	Project        string   `json:"project,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Location       string   `json:"location,omitempty"`
	People         []string `json:"people,omitempty"`
	Confidence     float64  `json:"confidence"`
	Ambiguities    []string `json:"ambiguities,omitempty"`
}

type ParseResult struct {
	Items []Candidate `json:"items"`
}

type Item struct {
	ID             string   `json:"id"`
	CaptureID      string   `json:"captureId,omitempty"`
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	StartAt        string   `json:"startAt,omitempty"`
	EndAt          string   `json:"endAt,omitempty"`
	DueAt          string   `json:"dueAt,omitempty"`
	ReminderAt     string   `json:"reminderAt,omitempty"`
	Timezone       string   `json:"timezone"`
	AllDay         bool     `json:"allDay"`
	RecurrenceRule string   `json:"recurrenceRule,omitempty"`
	Priority       int      `json:"priority"`
	Status         string   `json:"status"`
	Project        string   `json:"project,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Location       string   `json:"location,omitempty"`
	People         []string `json:"people,omitempty"`
	Version        int      `json:"version"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
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
