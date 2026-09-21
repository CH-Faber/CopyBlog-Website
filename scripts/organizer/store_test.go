package main

import (
	"path/filepath"
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

func TestSettingsLifecycle(t *testing.T) {
	store := testStore(t)
	if value, ok, err := store.setting("test_setting"); err != nil || ok || value != "" {
		t.Fatalf("unexpected missing setting: value=%q ok=%t err=%v", value, ok, err)
	}
	if err := store.setSetting("test_setting", "first"); err != nil {
		t.Fatal(err)
	}
	if err := store.setSetting("test_setting", "second"); err != nil {
		t.Fatal(err)
	}
	if value, ok, err := store.setting("test_setting"); err != nil || !ok || value != "second" {
		t.Fatalf("unexpected saved setting: value=%q ok=%t err=%v", value, ok, err)
	}
	if err := store.deleteSetting("test_setting"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.setting("test_setting"); err != nil || ok {
		t.Fatalf("setting was not deleted: ok=%t err=%v", ok, err)
	}
}
