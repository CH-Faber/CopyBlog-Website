package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxAssistantContextItems    = 50
	maxAssistantContextProjects = 50
	maxAssistantContextEvents   = 100
	maxAssistantContextMemories = 20
	maxAssistantTargetMatches   = 25
	maxAssistantMemoryText      = 500
)

type AssistantContextOptions struct {
	ItemLimit    int
	ProjectLimit int
	EventLimit   int
	MemoryLimit  int
}

type AssistantContext struct {
	Items    []AssistantContextItem    `json:"items"`
	Projects []AssistantContextProject `json:"projects"`
	Statuses []AssistantStatusCount    `json:"statuses"`
	Events   []AssistantContextEvent   `json:"events"`
	Memories []AssistantContextMemory  `json:"memories"`
}

type AssistantContextItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	StartAt   string `json:"startAt,omitempty"`
	DueAt     string `json:"dueAt,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Project   string `json:"project,omitempty"`
}

type AssistantContextProject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Deadline string `json:"deadline,omitempty"`
}

type AssistantStatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type AssistantContextEvent struct {
	ItemID    string `json:"itemId"`
	EventType string `json:"eventType"`
	CreatedAt string `json:"createdAt"`
}

type AssistantContextMemory struct {
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

type AssistantTargetCandidate struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Date    string `json:"date,omitempty"`
	Project string `json:"project,omitempty"`
	Status  string `json:"status"`
}

type AssistantTargetResolution struct {
	Resolved    bool                       `json:"resolved"`
	NeedsChoice bool                       `json:"needsChoice"`
	Target      *AssistantTargetCandidate  `json:"target,omitempty"`
	Candidates  []AssistantTargetCandidate `json:"candidates,omitempty"`
	Message     string                     `json:"message,omitempty"`
}

// assistantContext returns small, purpose-built records. It never reads capture
// rows or attachment metadata and every query is workspace-scoped and limited.
func (s *Store) assistantContext(workspaceID string, options AssistantContextOptions) (AssistantContext, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return AssistantContext{}, errors.New("workspace ID is required")
	}
	options.ItemLimit = boundedAssistantLimit(options.ItemLimit, maxAssistantContextItems)
	options.ProjectLimit = boundedAssistantLimit(options.ProjectLimit, maxAssistantContextProjects)
	options.EventLimit = boundedAssistantLimit(options.EventLimit, maxAssistantContextEvents)
	options.MemoryLimit = boundedAssistantLimit(options.MemoryLimit, maxAssistantContextMemories)
	ctx := AssistantContext{
		Items: make([]AssistantContextItem, 0), Projects: make([]AssistantContextProject, 0),
		Statuses: make([]AssistantStatusCount, 0), Events: make([]AssistantContextEvent, 0),
		Memories: make([]AssistantContextMemory, 0),
	}

	rows, err := s.db.Query(`SELECT i.id, i.title, i.status, i.start_at, i.due_at,
COALESCE(i.project_id, ''), COALESCE(p.name, i.project, '')
FROM items i LEFT JOIN projects p ON p.id=i.project_id AND p.workspace_id=i.workspace_id
WHERE i.workspace_id=?
ORDER BY CASE WHEN i.status IN ('done','cancelled','archived') THEN 1 ELSE 0 END,
CASE WHEN i.start_at<>'' THEN i.start_at WHEN i.due_at<>'' THEN i.due_at ELSE i.updated_at END,
i.id LIMIT ?`, workspaceID, options.ItemLimit)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var item AssistantContextItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Status, &item.StartAt, &item.DueAt, &item.ProjectID, &item.Project); err != nil {
			rows.Close()
			return ctx, err
		}
		ctx.Items = append(ctx.Items, item)
	}
	if err := rows.Close(); err != nil {
		return ctx, err
	}
	if err := rows.Err(); err != nil {
		return ctx, err
	}

	rows, err = s.db.Query(`SELECT p.id, p.name, p.status, p.deadline
FROM projects p WHERE p.workspace_id=?
ORDER BY CASE p.status WHEN 'active' THEN 0 WHEN 'waiting' THEN 1 WHEN 'blocked' THEN 2 ELSE 3 END,
p.updated_at DESC, p.id LIMIT ?`, workspaceID, options.ProjectLimit)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var project AssistantContextProject
		if err := rows.Scan(&project.ID, &project.Name, &project.Status, &project.Deadline); err != nil {
			rows.Close()
			return ctx, err
		}
		ctx.Projects = append(ctx.Projects, project)
	}
	if err := rows.Close(); err != nil {
		return ctx, err
	}
	if err := rows.Err(); err != nil {
		return ctx, err
	}

	rows, err = s.db.Query(`SELECT status, COUNT(*) FROM items WHERE workspace_id=? GROUP BY status ORDER BY status LIMIT 20`, workspaceID)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var count AssistantStatusCount
		if err := rows.Scan(&count.Status, &count.Count); err != nil {
			rows.Close()
			return ctx, err
		}
		ctx.Statuses = append(ctx.Statuses, count)
	}
	if err := rows.Close(); err != nil {
		return ctx, err
	}
	if err := rows.Err(); err != nil {
		return ctx, err
	}

	if len(ctx.Items) > 0 && options.EventLimit > 0 {
		args := []any{workspaceID}
		placeholders := make([]string, len(ctx.Items))
		for i, item := range ctx.Items {
			placeholders[i] = "?"
			args = append(args, item.ID)
		}
		args = append(args, options.EventLimit)
		query := `SELECT e.item_id, e.event_type, e.created_at FROM item_events e
JOIN items i ON i.id=e.item_id WHERE i.workspace_id=? AND e.item_id IN (` + strings.Join(placeholders, ",") + `)
ORDER BY e.created_at DESC, e.rowid DESC LIMIT ?`
		rows, err = s.db.Query(query, args...)
		if err != nil {
			return ctx, err
		}
		for rows.Next() {
			var event AssistantContextEvent
			if err := rows.Scan(&event.ItemID, &event.EventType, &event.CreatedAt); err != nil {
				rows.Close()
				return ctx, err
			}
			ctx.Events = append(ctx.Events, event)
		}
		if err := rows.Close(); err != nil {
			return ctx, err
		}
		if err := rows.Err(); err != nil {
			return ctx, err
		}
	}

	rows, err = s.db.Query(`SELECT kind, content FROM memories WHERE workspace_id=? AND status='active'
ORDER BY updated_at DESC, id LIMIT ?`, workspaceID, options.MemoryLimit)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var memory AssistantContextMemory
		if err := rows.Scan(&memory.Kind, &memory.Content); err != nil {
			rows.Close()
			return ctx, err
		}
		memory.Content = compactAssistantText(memory.Content, maxAssistantMemoryText)
		ctx.Memories = append(ctx.Memories, memory)
	}
	if err := rows.Close(); err != nil {
		return ctx, err
	}
	if err := rows.Err(); err != nil {
		return ctx, err
	}
	return ctx, nil
}

func boundedAssistantLimit(limit, maximum int) int {
	if limit <= 0 || limit > maximum {
		return maximum
	}
	return limit
}

func compactAssistantText(value string, maximum int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}

// resolveAssistantItemTarget resolves an exact title within a workspace. A
// project name and date phrase narrow the match; neither can broaden it.
func (s *Store) resolveAssistantItemTarget(workspaceID, title, projectName, datePhrase, timezone string, now time.Time) (AssistantTargetResolution, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return AssistantTargetResolution{}, errors.New("workspace ID is required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return AssistantTargetResolution{NeedsChoice: true, Message: "Which item do you mean?"}, nil
	}
	var projectID string
	if strings.TrimSpace(projectName) != "" {
		err := s.db.QueryRow(`SELECT id FROM projects WHERE workspace_id=? AND name=? COLLATE NOCASE LIMIT 1`, workspaceID, strings.TrimSpace(projectName)).Scan(&projectID)
		if errors.Is(err, sql.ErrNoRows) {
			return AssistantTargetResolution{NeedsChoice: true, Message: "No project matches that name."}, nil
		}
		if err != nil {
			return AssistantTargetResolution{}, err
		}
	}
	var dateRange *assistantDateRange
	if strings.TrimSpace(datePhrase) != "" {
		parsed, err := parseAssistantDatePhrase(datePhrase, timezone, now)
		if err != nil {
			return AssistantTargetResolution{}, err
		}
		dateRange = &parsed
	}

	query := `SELECT i.id, i.title, i.status, i.start_at, i.due_at, COALESCE(p.name, i.project, '')
FROM items i LEFT JOIN projects p ON p.id=i.project_id AND p.workspace_id=i.workspace_id
WHERE i.workspace_id=? AND i.title=? COLLATE NOCASE`
	args := []any{workspaceID, title}
	if projectID != "" {
		query += " AND i.project_id=?"
		args = append(args, projectID)
	}
	query += " ORDER BY i.id LIMIT ?"
	args = append(args, maxAssistantTargetMatches+1)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return AssistantTargetResolution{}, err
	}
	candidates := make([]AssistantTargetCandidate, 0)
	truncated := false
	seen := 0
	for rows.Next() {
		var id, candidateTitle, status, startAt, dueAt, project string
		if err := rows.Scan(&id, &candidateTitle, &status, &startAt, &dueAt, &project); err != nil {
			rows.Close()
			return AssistantTargetResolution{}, err
		}
		seen++
		if seen > maxAssistantTargetMatches {
			truncated = true
		}
		date := dueAt
		if date == "" {
			date = startAt
		}
		if dateRange != nil && !dateRange.contains(startAt, dueAt) {
			continue
		}
		candidates = append(candidates, AssistantTargetCandidate{ID: id, Title: candidateTitle, Date: date, Project: project, Status: status})
	}
	if err := rows.Close(); err != nil {
		return AssistantTargetResolution{}, err
	}
	if err := rows.Err(); err != nil {
		return AssistantTargetResolution{}, err
	}
	if truncated {
		if len(candidates) > maxAssistantTargetMatches {
			candidates = candidates[:maxAssistantTargetMatches]
		}
		return AssistantTargetResolution{NeedsChoice: true, Candidates: candidates, Message: "Too many items match to safely identify one. Please narrow the title or project."}, nil
	}
	if len(candidates) == 1 {
		return AssistantTargetResolution{Resolved: true, Target: &candidates[0]}, nil
	}
	if len(candidates) == 0 {
		return AssistantTargetResolution{NeedsChoice: true, Message: "No matching item was found."}, nil
	}
	if len(candidates) > maxAssistantTargetMatches {
		candidates = candidates[:maxAssistantTargetMatches]
	}
	return AssistantTargetResolution{NeedsChoice: true, Candidates: candidates, Message: "More than one item matches. Which one do you mean?"}, nil
}

type assistantDateRange struct {
	start time.Time
	end   time.Time
	loc   *time.Location
}

func parseAssistantDatePhrase(phrase, timezone string, now time.Time) (assistantDateRange, error) {
	loc := time.UTC
	if timezone != "" {
		loaded, err := time.LoadLocation(timezone)
		if err != nil {
			return assistantDateRange{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
		loc = loaded
	}
	if now.IsZero() {
		now = time.Now()
	}
	local := now.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	switch strings.ToLower(strings.TrimSpace(phrase)) {
	case "today":
	case "tomorrow":
		day = day.AddDate(0, 0, 1)
	case "yesterday":
		day = day.AddDate(0, 0, -1)
	default:
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(phrase), loc)
		if err != nil {
			return assistantDateRange{}, fmt.Errorf("unsupported date phrase %q", phrase)
		}
		day = parsed
	}
	return assistantDateRange{start: day, end: day.AddDate(0, 0, 1), loc: loc}, nil
}

func (r assistantDateRange) contains(startAt, dueAt string) bool {
	for _, value := range []string{startAt, dueAt} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err == nil {
			parsed = parsed.In(r.loc)
			if !parsed.Before(r.start) && parsed.Before(r.end) {
				return true
			}
			continue
		}
		if len(value) >= 10 {
			if day, err := time.ParseInLocation("2006-01-02", value[:10], r.loc); err == nil && !day.Before(r.start) && day.Before(r.end) {
				return true
			}
		}
	}
	return false
}
