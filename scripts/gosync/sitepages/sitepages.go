package sitepages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gosync/config"
)

type Page struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Heading     string `json:"heading"`
	Subtitle    string `json:"subtitle"`
}

type Pages struct {
	Version int    `json:"version"`
	Pages   []Page `json:"pages"`
}

func PublishedPath(cfg *config.Config) string {
	return filepath.Join(cfg.ProjectRootDir, "src", "data", "site-pages.json")
}

func DraftPath(cfg *config.Config) string {
	return filepath.Join(cfg.ProjectRootDir, ".gosync", "state", "site-pages.json")
}

func Load(cfg *config.Config) (*Pages, error) {
	path := DraftPath(cfg)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		path = PublishedPath(cfg)
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read page information %s: %w", path, err)
	}
	var value Pages
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse page information %s: %w", path, err)
	}
	if err := Validate(&value); err != nil {
		return nil, err
	}
	return &value, nil
}

func Validate(value *Pages) error {
	if value == nil || len(value.Pages) == 0 {
		return fmt.Errorf("page information cannot be empty")
	}
	if value.Version <= 0 {
		value.Version = 1
	}
	seen := map[string]bool{}
	for index := range value.Pages {
		page := &value.Pages[index]
		page.Key = strings.TrimSpace(page.Key)
		page.Name = strings.TrimSpace(page.Name)
		page.Title = strings.TrimSpace(page.Title)
		page.Description = strings.TrimSpace(page.Description)
		page.Heading = strings.TrimSpace(page.Heading)
		page.Subtitle = strings.TrimSpace(page.Subtitle)
		if page.Key == "" || page.Name == "" || page.Title == "" || page.Heading == "" {
			return fmt.Errorf("page key, name, title and heading are required")
		}
		if seen[page.Key] {
			return fmt.Errorf("duplicate page key: %s", page.Key)
		}
		seen[page.Key] = true
	}
	return nil
}

func JSON(value *Pages) ([]byte, error) {
	if err := Validate(value); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func SaveDraft(cfg *config.Config, value *Pages) error {
	data, err := JSON(value)
	if err != nil {
		return err
	}
	path := DraftPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func PublishedHash(cfg *config.Config) (string, error) {
	data, err := os.ReadFile(PublishedPath(cfg))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
