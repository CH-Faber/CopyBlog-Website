package taxonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gosync/config"
	"gosync/contentmodel"
)

type ManagedTag struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	Description  string   `json:"description"`
	Enabled      bool     `json:"enabled"`
	AISelectable bool     `json:"aiSelectable"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

type Category struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Enabled      bool   `json:"enabled"`
	AISelectable bool   `json:"aiSelectable"`
}

type Taxonomy struct {
	Version    int          `json:"version"`
	Categories []Category   `json:"categories"`
	Tags       []ManagedTag `json:"tags"`
}

type Usage struct {
	Tags       map[string]int `json:"tags"`
	Categories map[string]int `json:"categories"`
}

func stableID(prefix, name string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return prefix + "-" + hex.EncodeToString(sum[:6])
}

func NewTag(name string) ManagedTag {
	now := time.Now().UTC().Format(time.RFC3339)
	return ManagedTag{ID: stableID("tag", name), Name: strings.TrimSpace(name), Aliases: []string{}, Enabled: true, AISelectable: true, CreatedAt: now, UpdatedAt: now}
}

func NewCategory(name string) Category {
	return Category{ID: stableID("category", name), Name: strings.TrimSpace(name), Enabled: true, AISelectable: true}
}

func publishedPath(cfg *config.Config) string {
	return filepath.Join(cfg.ProjectRootDir, "src", "data", "content-taxonomy.json")
}

func draftPath(cfg *config.Config) string {
	return filepath.Join(cfg.ProjectRootDir, ".gosync", "state", "taxonomy.json")
}

func Load(cfg *config.Config) (*Taxonomy, error) {
	for _, path := range []string{draftPath(cfg), publishedPath(cfg)} {
		data, err := os.ReadFile(path)
		if err == nil {
			var value Taxonomy
			if err := json.Unmarshal(data, &value); err != nil {
				return nil, fmt.Errorf("parse taxonomy %s: %w", path, err)
			}
			normalize(&value)
			return &value, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	value, err := SeedFromPosts(cfg.LocalPostsDir)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func SaveDraft(cfg *config.Config, value *Taxonomy) error {
	normalize(value)
	if err := os.MkdirAll(filepath.Dir(draftPath(cfg)), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp := draftPath(cfg) + ".tmp"
	if err := os.WriteFile(temp, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(temp, draftPath(cfg))
}

func PublishedJSON(value *Taxonomy) ([]byte, error) {
	normalize(value)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func SeedFromPosts(postsDir string) (*Taxonomy, error) {
	tags := map[string]struct{}{}
	categories := map[string]struct{}{}
	entries, err := os.ReadDir(postsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") && !strings.EqualFold(filepath.Ext(entry.Name()), ".mdx") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(postsDir, entry.Name()))
		if err != nil {
			continue
		}
		doc, err := contentmodel.Parse(string(data))
		if err != nil {
			continue
		}
		if name := strings.TrimSpace(doc.Metadata.Category); name != "" {
			categories[name] = struct{}{}
		}
		for _, name := range doc.Metadata.Tags {
			if name = strings.TrimSpace(name); name != "" {
				tags[name] = struct{}{}
			}
		}
	}
	result := &Taxonomy{Version: 2, Categories: []Category{}, Tags: []ManagedTag{}}
	for name := range categories {
		result.Categories = append(result.Categories, NewCategory(name))
	}
	for name := range tags {
		result.Tags = append(result.Tags, NewTag(name))
	}
	normalize(result)
	return result, nil
}

// CountUsage reports how many published source articles use each canonical tag.
// Aliases are resolved to their canonical tag so a renamed tag still has a useful count.
func CountUsage(postsDir string, value *Taxonomy) (*Usage, error) {
	result := &Usage{Tags: map[string]int{}, Categories: map[string]int{}}
	for _, tag := range value.Tags {
		result.Tags[tag.Name] = 0
	}
	categoryNames := map[string]string{}
	for _, category := range value.Categories {
		result.Categories[category.Name] = 0
		categoryNames[strings.ToLower(category.Name)] = category.Name
	}
	allowed := map[string]string{}
	for _, tag := range value.Tags {
		allowed[strings.ToLower(tag.Name)] = tag.Name
		for _, alias := range tag.Aliases {
			if alias = strings.TrimSpace(alias); alias != "" {
				allowed[strings.ToLower(alias)] = tag.Name
			}
		}
	}
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") && !strings.EqualFold(filepath.Ext(entry.Name()), ".mdx") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(postsDir, entry.Name()))
		if err != nil {
			continue
		}
		doc, err := contentmodel.Parse(string(data))
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		if canonical, ok := categoryNames[strings.ToLower(strings.TrimSpace(doc.Metadata.Category))]; ok {
			result.Categories[canonical]++
		}
		for _, raw := range doc.Metadata.Tags {
			canonical, ok := allowed[strings.ToLower(strings.TrimSpace(raw))]
			if ok && !seen[canonical] {
				result.Tags[canonical]++
				seen[canonical] = true
			}
		}
	}
	return result, nil
}

func normalize(value *Taxonomy) {
	legacyCategories := value.Version < 2
	value.Version = 2
	now := time.Now().UTC().Format(time.RFC3339)
	seen := map[string]bool{}
	cleanTags := make([]ManagedTag, 0, len(value.Tags))
	for _, tag := range value.Tags {
		tag.Name = strings.TrimSpace(tag.Name)
		if tag.Name == "" || seen[strings.ToLower(tag.Name)] {
			continue
		}
		seen[strings.ToLower(tag.Name)] = true
		if tag.ID == "" {
			tag.ID = stableID("tag", tag.Name)
		}
		if tag.CreatedAt == "" {
			tag.CreatedAt = now
		}
		tag.UpdatedAt = now
		if tag.Aliases == nil {
			tag.Aliases = []string{}
		}
		if !tag.Enabled {
			tag.AISelectable = false
		}
		cleanTags = append(cleanTags, tag)
	}
	value.Tags = cleanTags
	seenCategories := map[string]bool{}
	cleanCategories := make([]Category, 0, len(value.Categories))
	for _, category := range value.Categories {
		category.Name = strings.TrimSpace(category.Name)
		key := strings.ToLower(category.Name)
		if category.Name == "" || seenCategories[key] {
			continue
		}
		seenCategories[key] = true
		if category.ID == "" {
			category.ID = stableID("category", category.Name)
		}
		if legacyCategories {
			category.AISelectable = category.Enabled
		}
		if !category.Enabled {
			category.AISelectable = false
		}
		cleanCategories = append(cleanCategories, category)
	}
	value.Categories = cleanCategories
	sort.Slice(value.Tags, func(i, j int) bool { return value.Tags[i].Name < value.Tags[j].Name })
	sort.Slice(value.Categories, func(i, j int) bool { return value.Categories[i].Name < value.Categories[j].Name })
}

func (value *Taxonomy) AllowedCategoryNames() map[string]string {
	result := map[string]string{}
	for _, category := range value.Categories {
		if category.Enabled {
			result[strings.ToLower(category.Name)] = category.Name
		}
	}
	return result
}

func (value *Taxonomy) AllowedAICategories() []Category {
	result := []Category{}
	for _, category := range value.Categories {
		if category.Enabled && category.AISelectable {
			result = append(result, category)
		}
	}
	return result
}

func (value *Taxonomy) ValidateCategory(input string) (string, bool) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", false
	}
	canonical, ok := value.AllowedCategoryNames()[strings.ToLower(name)]
	return canonical, ok
}

func (value *Taxonomy) AllowedTags() []ManagedTag {
	result := []ManagedTag{}
	for _, tag := range value.Tags {
		if tag.Enabled && tag.AISelectable {
			result = append(result, tag)
		}
	}
	return result
}

func (value *Taxonomy) AllowedTagNames() map[string]string {
	result := map[string]string{}
	for _, tag := range value.Tags {
		if !tag.Enabled {
			continue
		}
		result[strings.ToLower(tag.Name)] = tag.Name
		for _, alias := range tag.Aliases {
			result[strings.ToLower(strings.TrimSpace(alias))] = tag.Name
		}
	}
	return result
}

func (value *Taxonomy) ValidateTags(input []string) (valid []string, unknown []string) {
	allowed := value.AllowedTagNames()
	seen := map[string]bool{}
	for _, raw := range input {
		name := strings.TrimSpace(raw)
		canonical, ok := allowed[strings.ToLower(name)]
		if !ok {
			if name != "" {
				unknown = append(unknown, name)
			}
			continue
		}
		if !seen[canonical] {
			valid = append(valid, canonical)
			seen[canonical] = true
		}
	}
	return valid, unknown
}
