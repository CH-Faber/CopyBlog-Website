package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

type recurrenceRule struct {
	Frequency string `json:"frequency"`
	Interval  int    `json:"interval"`
	Weekdays  []int  `json:"weekdays,omitempty"`
	Day       int    `json:"day,omitempty"`
	Count     int    `json:"count,omitempty"`
	Until     string `json:"until,omitempty"`
}

func validateRecurrenceRule(raw json.RawMessage, zone, kind string) (recurrenceRule, error) {
	var rule recurrenceRule
	if len(raw) == 0 || !json.Valid(raw) {
		return rule, errors.New("rule must be valid JSON")
	}
	if err := json.Unmarshal(raw, &rule); err != nil {
		return rule, err
	}
	if rule.Interval == 0 {
		rule.Interval = 1
	}
	if rule.Interval < 1 || rule.Interval > 366 {
		return rule, errors.New("rule.interval must be between 1 and 366")
	}
	switch rule.Frequency {
	case "daily":
	case "weekly":
		if len(rule.Weekdays) == 0 {
			return rule, errors.New("weekly rule requires weekdays")
		}
		seen := map[int]bool{}
		for _, day := range rule.Weekdays {
			if day < 0 || day > 6 || seen[day] {
				return rule, errors.New("weekdays must contain unique values from 0 to 6")
			}
			seen[day] = true
		}
	case "monthly":
		if rule.Day < 1 || rule.Day > 31 {
			return rule, errors.New("monthly rule.day must be between 1 and 31")
		}
	default:
		return rule, errors.New("rule.frequency must be daily, weekly, or monthly")
	}
	if rule.Count < 0 {
		return rule, errors.New("rule.count cannot be negative")
	}
	if rule.Until != "" {
		until, err := time.Parse(time.RFC3339, rule.Until)
		if err != nil {
			return rule, errors.New("rule.until must be RFC3339")
		}
		if until.Location() == time.UTC && zone != "UTC" {
			// RFC3339 offsets are still valid; only validate the location below.
		}
	}
	if kind != "event" && kind != "task" {
		return rule, errors.New("series kind must be event or task")
	}
	return rule, nil
}

func (s *Store) createRecurrenceSeries(series RecurrenceSeries) (RecurrenceSeries, error) {
	if series.ItemID == "" || series.StartsAt == "" {
		return RecurrenceSeries{}, errors.New("itemId and startsAt are required")
	}
	item, err := s.getItem(series.ItemID)
	if err != nil {
		return RecurrenceSeries{}, err
	}
	if series.Kind == "" {
		series.Kind = item.Type
	}
	if series.Timezone == "" {
		series.Timezone = item.Timezone
	}
	if _, err := time.LoadLocation(series.Timezone); err != nil {
		return RecurrenceSeries{}, errors.New("invalid timezone")
	}
	if _, err := time.Parse(time.RFC3339, series.StartsAt); err != nil {
		return RecurrenceSeries{}, errors.New("startsAt must be RFC3339")
	}
	if series.EndsAt != "" {
		end, err := time.Parse(time.RFC3339, series.EndsAt)
		if err != nil {
			return RecurrenceSeries{}, errors.New("endsAt must be RFC3339")
		}
		start, _ := time.Parse(time.RFC3339, series.StartsAt)
		if end.Before(start) {
			return RecurrenceSeries{}, errors.New("endsAt must not be before startsAt")
		}
	}
	rule, err := validateRecurrenceRule(series.Rule, series.Timezone, series.Kind)
	if err != nil {
		return RecurrenceSeries{}, err
	}
	encoded, _ := json.Marshal(rule)
	now := nowString()
	series.ID, series.WorkspaceID, series.RuleVersion, series.Rule, series.Status, series.CreatedAt, series.UpdatedAt = newID("series"), s.defaultWorkspaceID(), 1, encoded, "active", now, now
	_, err = s.db.Exec(`INSERT INTO recurrence_series(id, workspace_id, item_id, kind, rule_version, rule_json, timezone, starts_at, ends_at, status, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, series.ID, series.WorkspaceID, series.ItemID, series.Kind, series.RuleVersion, string(series.Rule), series.Timezone, series.StartsAt, series.EndsAt, series.Status, now, now)
	if err != nil {
		return RecurrenceSeries{}, err
	}
	return series, nil
}

func (s *Store) getRecurrenceSeries(id string) (RecurrenceSeries, error) {
	var series RecurrenceSeries
	var rule string
	err := s.db.QueryRow(`SELECT id, workspace_id, item_id, kind, rule_version, rule_json, timezone, starts_at, ends_at, status, created_at, updated_at FROM recurrence_series WHERE id=? AND workspace_id=?`, id, s.defaultWorkspaceID()).Scan(&series.ID, &series.WorkspaceID, &series.ItemID, &series.Kind, &series.RuleVersion, &rule, &series.Timezone, &series.StartsAt, &series.EndsAt, &series.Status, &series.CreatedAt, &series.UpdatedAt)
	if err != nil {
		return series, err
	}
	series.Rule = json.RawMessage(rule)
	return series, nil
}

func nextRecurrence(t time.Time, rule recurrenceRule) time.Time {
	switch rule.Frequency {
	case "daily":
		return t.AddDate(0, 0, rule.Interval)
	case "weekly":
		if rule.Interval == 1 {
			for day := 1; day <= 7; day++ {
				candidate := t.AddDate(0, 0, day)
				if containsInt(rule.Weekdays, int(candidate.Weekday())) {
					return candidate
				}
			}
		}
		candidate := t.AddDate(0, 0, 7*rule.Interval)
		for day := 0; day < 7; day++ {
			if containsInt(rule.Weekdays, int(candidate.Weekday())) {
				return candidate
			}
			candidate = candidate.AddDate(0, 0, 1)
		}
		return t.AddDate(0, 0, 7*rule.Interval)
	case "monthly":
		candidate := t.AddDate(0, rule.Interval, 0)
		day := rule.Day
		last := time.Date(candidate.Year(), candidate.Month()+1, 0, candidate.Hour(), candidate.Minute(), candidate.Second(), candidate.Nanosecond(), candidate.Location()).Day()
		if day > last {
			day = last
		}
		return time.Date(candidate.Year(), candidate.Month(), day, candidate.Hour(), candidate.Minute(), candidate.Second(), candidate.Nanosecond(), candidate.Location())
	default:
		return t
	}
}

func containsInt(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func (s *Store) listRecurrenceOccurrences(seriesID, fromText, toText string) ([]RecurrenceOccurrence, error) {
	series, err := s.getRecurrenceSeries(seriesID)
	if err != nil {
		return nil, err
	}
	from, err := parseRangeTime(fromText, time.Now().AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	to, err := parseRangeTime(toText, time.Now().AddDate(0, 0, 90))
	if err != nil {
		return nil, err
	}
	if !to.After(from) {
		return nil, errors.New("to must be after from")
	}
	var rule recurrenceRule
	if err := json.Unmarshal(series.Rule, &rule); err != nil {
		return nil, err
	}
	startInstant, _ := time.Parse(time.RFC3339, series.StartsAt)
	location, err := time.LoadLocation(series.Timezone)
	if err != nil {
		return nil, errors.New("invalid timezone")
	}
	start := startInstant.In(location)
	endLimit := to
	if series.EndsAt != "" {
		if value, _ := time.Parse(time.RFC3339, series.EndsAt); value.Before(endLimit) {
			endLimit = value
		}
	}
	values := []RecurrenceOccurrence{}
	generated := 0
	for offset := 0; offset <= 36600; offset++ {
		current := start.AddDate(0, 0, offset)
		if current.After(endLimit) || generated >= 1000 {
			break
		}
		if rule.Until != "" {
			until, _ := time.Parse(time.RFC3339, rule.Until)
			if current.After(until) {
				break
			}
		}
		matches := false
		switch rule.Frequency {
		case "daily":
			matches = offset%rule.Interval == 0
		case "weekly":
			anchorWeekday := (int(start.Weekday()) + 6) % 7
			weekIndex := (offset + anchorWeekday) / 7
			matches = weekIndex%rule.Interval == 0 && containsInt(rule.Weekdays, int(current.Weekday()))
		case "monthly":
			monthDelta := (current.Year()-start.Year())*12 + int(current.Month()-start.Month())
			day := rule.Day
			last := time.Date(current.Year(), current.Month()+1, 0, 0, 0, 0, 0, current.Location()).Day()
			if day > last {
				day = last
			}
			matches = monthDelta%rule.Interval == 0 && current.Day() == day
		}
		if !matches {
			continue
		}
		if rule.Count > 0 && generated >= rule.Count {
			break
		}
		generated++
		if !current.Before(from) && current.Before(to) {
			key := current.UTC().Format(time.RFC3339)
			occ, err := s.ensureOccurrence(series, key, current)
			if err != nil {
				return nil, err
			}
			if occ.Status != "skipped" && occ.Status != "cancelled" {
				values = append(values, occ)
			}
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ScheduledStart < values[j].ScheduledStart })
	return values, nil
}

func parseRangeTime(text string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(text) == "" {
		return fallback, nil
	}
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, errors.New("range must be RFC3339")
	}
	return value, nil
}

func (s *Store) ensureOccurrence(series RecurrenceSeries, key string, start time.Time) (RecurrenceOccurrence, error) {
	var occ RecurrenceOccurrence
	var override string
	err := s.db.QueryRow(`SELECT id, series_id, occurrence_key, scheduled_start, scheduled_end, status, override_json, completed_at, version FROM recurrence_occurrences WHERE series_id=? AND occurrence_key=?`, series.ID, key).Scan(&occ.ID, &occ.SeriesID, &occ.OccurrenceKey, &occ.ScheduledStart, &occ.ScheduledEnd, &occ.Status, &override, &occ.CompletedAt, &occ.Version)
	if err == nil {
		if override != "" {
			occ.Override = json.RawMessage(override)
		}
		return occ, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return occ, err
	}
	item, err := s.getItem(series.ItemID)
	if err != nil {
		return occ, err
	}
	end := ""
	if item.EndAt != "" {
		base, _ := time.Parse(time.RFC3339, item.StartAt)
		finish, _ := time.Parse(time.RFC3339, item.EndAt)
		end = start.Add(finish.Sub(base)).UTC().Format(time.RFC3339)
	}
	now := nowString()
	occ = RecurrenceOccurrence{ID: newID("occurrence"), SeriesID: series.ID, OccurrenceKey: key, ScheduledStart: start.UTC().Format(time.RFC3339), ScheduledEnd: end, Status: "scheduled", Version: 1}
	_, err = s.db.Exec(`INSERT INTO recurrence_occurrences(id, workspace_id, series_id, occurrence_key, scheduled_start, scheduled_end, status, version, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, 'scheduled', 1, ?, ?)`, occ.ID, s.defaultWorkspaceID(), series.ID, key, occ.ScheduledStart, end, now, now)
	return occ, err
}

func (s *Store) updateOccurrence(id, action string, start, end string, override json.RawMessage, expected int) (RecurrenceOccurrence, error) {
	var occ RecurrenceOccurrence
	var existingOverride string
	err := s.db.QueryRow(`SELECT id, series_id, occurrence_key, scheduled_start, scheduled_end, status, override_json, completed_at, version FROM recurrence_occurrences WHERE id=? AND workspace_id=?`, id, s.defaultWorkspaceID()).Scan(&occ.ID, &occ.SeriesID, &occ.OccurrenceKey, &occ.ScheduledStart, &occ.ScheduledEnd, &occ.Status, &existingOverride, &occ.CompletedAt, &occ.Version)
	if err != nil {
		return occ, err
	}
	if expected > 0 && occ.Version != expected {
		return occ, errConflict
	}
	if action != "skip" && action != "move" && action != "cancel" && action != "complete" {
		return occ, errors.New("action must be skip, move, cancel, or complete")
	}
	if action == "move" {
		if _, err := time.Parse(time.RFC3339, start); err != nil {
			return occ, errors.New("scheduledStart must be RFC3339")
		}
		occ.ScheduledStart = start
		if end != "" {
			occ.ScheduledEnd = end
		}
	}
	if action == "skip" || action == "cancel" {
		occ.Status = action + "ped"
		if action == "cancel" {
			occ.Status = "cancelled"
		}
	}
	if action == "complete" {
		occ.Status, occ.CompletedAt = "completed", nowString()
	}
	if len(override) > 0 {
		existingOverride = string(override)
		occ.Override = override
	}
	occ.Version++
	_, err = s.db.Exec(`UPDATE recurrence_occurrences SET scheduled_start=?, scheduled_end=?, status=?, override_json=?, completed_at=?, version=?, updated_at=? WHERE id=? AND workspace_id=? AND version=?`, occ.ScheduledStart, occ.ScheduledEnd, occ.Status, existingOverride, occ.CompletedAt, occ.Version, nowString(), id, s.defaultWorkspaceID(), occ.Version-1)
	return occ, err
}

func (s *Server) handleCreateRecurrenceSeries(w http.ResponseWriter, r *http.Request) {
	var series RecurrenceSeries
	if err := decodeJSON(r, &series, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.createRecurrenceSeries(series)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetRecurrenceSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.store.getRecurrenceSeries(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "series not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get series")
		return
	}
	writeJSON(w, http.StatusOK, series)
}

func (s *Server) handleListRecurrenceOccurrences(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.listRecurrenceOccurrences(r.PathValue("id"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "series not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"occurrences": values})
}

func (s *Server) handleUpdateRecurrenceOccurrence(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Action          string          `json:"action"`
		ScheduledStart  string          `json:"scheduledStart"`
		ScheduledEnd    string          `json:"scheduledEnd"`
		Override        json.RawMessage `json:"override"`
		ExpectedVersion int             `json:"expectedVersion"`
	}
	if err := decodeJSON(r, &request, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	value, err := s.store.updateOccurrence(r.PathValue("id"), request.Action, request.ScheduledStart, request.ScheduledEnd, request.Override, request.ExpectedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "occurrence not found")
		return
	}
	if errors.Is(err, errConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, value)
}
