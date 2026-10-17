package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

func TestRecurrenceRuleValidation(t *testing.T) {
	tests := []struct {
		name  string
		rule  string
		valid bool
	}{
		{"daily", `{"frequency":"daily"}`, true},
		{"weekly weekdays", `{"frequency":"weekly","weekdays":[1,3,5]}`, true},
		{"monthly day", `{"frequency":"monthly","day":31}`, true},
		{"weekly missing days", `{"frequency":"weekly"}`, false},
		{"invalid weekday", `{"frequency":"weekly","weekdays":[7]}`, false},
		{"unknown frequency", `{"frequency":"hourly"}`, false},
		{"invalid count", `{"frequency":"daily","count":-1}`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateRecurrenceRule(json.RawMessage(test.rule), "Asia/Shanghai", "event")
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}

func TestRecurrenceOccurrencesPreserveInstanceExceptions(t *testing.T) {
	store := testStore(t)
	items, err := store.createItems("", []Candidate{{Type: "event", Title: "Class", StartAt: "2026-10-19T19:00:00+08:00", EndAt: "2026-10-19T20:00:00+08:00", Timezone: "Asia/Shanghai"}}, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	series, err := store.createRecurrenceSeries(RecurrenceSeries{
		ItemID:   items[0].ID,
		Kind:     "event",
		Rule:     json.RawMessage(`{"frequency":"weekly","weekdays":[1,3,5]}`),
		Timezone: "Asia/Shanghai",
		StartsAt: "2026-10-19T19:00:00+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	values, err := store.listRecurrenceOccurrences(series.ID, "2026-10-19T00:00:00+08:00", "2026-10-27T00:00:00+08:00")
	if err != nil || len(values) != 4 {
		t.Fatalf("occurrences=%d err=%v", len(values), err)
	}
	first := values[0]
	if first.ScheduledEnd != "2026-10-19T12:00:00Z" {
		t.Fatalf("end did not retain duration: %s", first.ScheduledEnd)
	}
	skipped, err := store.updateOccurrence(first.ID, "skip", "", "", nil, first.Version)
	if err != nil || skipped.Status != "skipped" {
		t.Fatalf("skip failed: %#v %v", skipped, err)
	}
	if _, err := store.updateOccurrence(first.ID, "complete", "", "", nil, first.Version); !errors.Is(err, errConflict) {
		t.Fatalf("stale occurrence update err=%v", err)
	}
	values, err = store.listRecurrenceOccurrences(series.ID, "2026-10-19T00:00:00+08:00", "2026-10-27T00:00:00+08:00")
	if err != nil || len(values) != 3 {
		t.Fatalf("skip did not suppress one instance: count=%d err=%v", len(values), err)
	}
	var stored int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM recurrence_occurrences WHERE series_id=?", series.ID).Scan(&stored); err != nil || stored != 4 {
		t.Fatalf("generated occurrence persistence=%d err=%v", stored, err)
	}
	if _, err := store.getRecurrenceSeries("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing series error=%v", err)
	}
}

func TestMigrationCreatesRecurrenceTablesIdempotently(t *testing.T) {
	store := testStore(t)
	if err := store.migrate(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"recurrence_series", "recurrence_occurrences", "recurrence_exceptions"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table=%s count=%d err=%v", table, count, err)
		}
	}
	var migrations int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=11").Scan(&migrations); err != nil || migrations != 1 {
		t.Fatalf("version 11 count=%d err=%v", migrations, err)
	}
}
