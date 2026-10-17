package main

import (
	"errors"
	"regexp"
	"strings"
)

var durableMemoryPrefix = regexp.MustCompile(`(?:以后|今后|记住|总是|一直)\s*`)
var sensitiveMemoryPattern = regexp.MustCompile(`(?i)(密码|口令|token|密钥|银行卡|身份证|病历|医疗|性生活|性取向|法律案件)`)

// extractDurableMemory accepts only explicit durable instructions. A normal
// one-off task or an assistant inference must never become persistent memory.
func extractDurableMemory(text string) (Memory, bool) {
	content := strings.TrimSpace(text)
	if content == "" || sensitiveMemoryPattern.MatchString(content) || !durableMemoryPrefix.MatchString(content) {
		return Memory{}, false
	}
	content = durableMemoryPrefix.ReplaceAllString(content, "")
	content = strings.TrimSpace(strings.Trim(content, "，。！？,.!? "))
	if len([]rune(content)) < 4 || len([]rune(content)) > 160 {
		return Memory{}, false
	}
	key := strings.ToLower(strings.Join(strings.Fields(content), " "))
	return Memory{Kind: "preference", Key: key, Content: content, Scope: "global", Status: "active", EvidenceCount: 1}, true
}

func (s *Store) learnDurableMemory(text string) error {
	memory, ok := extractDurableMemory(text)
	if !ok {
		return nil
	}
	memories, err := s.listMemories("")
	if err != nil {
		return err
	}
	for _, existing := range memories {
		if existing.Key == memory.Key && existing.Status != "forgotten" {
			existing.Content = memory.Content
			existing.Status = "active"
			existing.EvidenceCount++
			_, err = s.updateMemory(existing)
			return err
		}
	}
	_, err = s.createMemory(memory)
	return err
}

var errSensitiveMemory = errors.New("sensitive content is not eligible for memory")
