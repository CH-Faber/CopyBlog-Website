package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "organizer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestCaptureToItemLifecycle(t *testing.T) {
	store := testStore(t)
	capture, err := store.createCapture(Capture{SourceType: "text", RawText: "明天交报告"})
	if err != nil {
		t.Fatal(err)
	}
	result := ParseResult{Items: []Candidate{{Type: "task", Title: "提交报告", DueAt: "2026-09-22T18:00:00+08:00", ReminderAt: "2026-09-22T17:30:00+08:00", Timezone: "Asia/Shanghai"}}}
	if err := store.setCaptureResult(capture.ID, result); err != nil {
		t.Fatal(err)
	}
	items, err := store.createItems(capture.ID, result.Items, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "提交报告" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if items[0].DueAt != "2026-09-22T10:00:00Z" {
		t.Fatalf("time was not normalized: %s", items[0].DueAt)
	}
	confirmed, err := store.getCapture(capture.ID)
	if err != nil || confirmed.Status != "confirmed" {
		t.Fatalf("capture not confirmed: %#v, %v", confirmed, err)
	}
	completed, err := store.completeItem(items[0].ID)
	if err != nil || completed.Status != "done" || completed.Version != 2 {
		t.Fatalf("item not completed: %#v, %v", completed, err)
	}
}

func TestDirectItemDoesNotRequireCapture(t *testing.T) {
	store := testStore(t)
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "独立事项"}}, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CaptureID != "" {
		t.Fatalf("unexpected item: %#v", items)
	}
}

func TestItemHistoryProjectAndRestore(t *testing.T) {
	store := testStore(t)
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "完成项目首页", Project: "网站改版", Certainty: "tentative", DurationMinutes: 90}}, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]
	if item.ProjectID == "" || item.Certainty != "tentative" || item.DurationMinutes != 90 {
		t.Fatalf("extended item fields were not stored: %#v", item)
	}
	projects, err := store.listProjects()
	if err != nil || len(projects) != 1 || projects[0].OpenCount != 1 {
		t.Fatalf("project was not created: %#v %v", projects, err)
	}
	completed, err := store.completeItem(item.ID)
	if err != nil || completed.Status != "done" || completed.CompletedAt == "" {
		t.Fatalf("completion was not retained: %#v %v", completed, err)
	}
	reopened, err := store.reopenItem(item.ID)
	if err != nil || reopened.Status != "todo" || reopened.CompletedAt != "" {
		t.Fatalf("item was not restored: %#v %v", reopened, err)
	}
	events, err := store.listItemEvents(item.ID, 20)
	if err != nil || len(events) != 3 || events[0].EventType != "reopened" {
		t.Fatalf("unexpected history: %#v %v", events, err)
	}
}

func TestMemoryApprovalAndPrompt(t *testing.T) {
	store := testStore(t)
	memory, err := store.createMemory(Memory{Kind: "preference", Content: "早上默认安排在 08:30。", Status: "active"})
	if err != nil || memory.ID == "" {
		t.Fatalf("create memory failed: %#v %v", memory, err)
	}
	prompt, err := store.activeMemoryPrompt()
	if err != nil || !strings.Contains(prompt, "08:30") {
		t.Fatalf("memory was not available to the assistant: %q %v", prompt, err)
	}
	memory.Status = "forgotten"
	if _, err := store.updateMemory(memory); err != nil {
		t.Fatal(err)
	}
	prompt, err = store.activeMemoryPrompt()
	if err != nil || prompt != "" {
		t.Fatalf("forgotten memory remained active: %q %v", prompt, err)
	}
}
