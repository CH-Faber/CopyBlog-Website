package main

import "testing"

func TestExtractDurableMemoryRequiresExplicitDurableLanguage(t *testing.T) {
	if _, ok := extractDurableMemory("明天下午三点开会"); ok {
		t.Fatal("one-off task became memory")
	}
	memory, ok := extractDurableMemory("以后 工作日晚上不要安排工作")
	if !ok || memory.Status != "active" || memory.Content == "" {
		t.Fatalf("durable instruction was not accepted: %#v", memory)
	}
}

func TestExtractDurableMemoryRejectsSensitiveContent(t *testing.T) {
	if _, ok := extractDurableMemory("记住我的密码是 secret"); ok {
		t.Fatal("sensitive memory was accepted")
	}
}

func TestLearnDurableMemoryDeduplicatesAndIncrementsEvidence(t *testing.T) {
	store := testStore(t)
	if err := store.learnDurableMemory("以后 默认使用简洁标题"); err != nil {
		t.Fatal(err)
	}
	if err := store.learnDurableMemory("记住默认使用简洁标题"); err != nil {
		t.Fatal(err)
	}
	memories, err := store.listMemories("active")
	if err != nil || len(memories) != 1 || memories[0].EvidenceCount != 2 {
		t.Fatalf("memory dedupe failed: %#v %v", memories, err)
	}
}
