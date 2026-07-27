package taxonomy

import "testing"

func TestValidateTagsUsesAliasesAndRejectsUnknown(t *testing.T) {
	value := &Taxonomy{Version: 1, Tags: []ManagedTag{
		{Name: "自我成长", Aliases: []string{"个人成长"}, Enabled: true, AISelectable: true},
		{Name: "停用标签", Enabled: false, AISelectable: true},
	}}
	valid, unknown := value.ValidateTags([]string{"个人成长", "自我成长", "新标签", "停用标签"})
	if len(valid) != 1 || valid[0] != "自我成长" {
		t.Fatalf("unexpected valid tags: %#v", valid)
	}
	if len(unknown) != 2 {
		t.Fatalf("unexpected unknown tags: %#v", unknown)
	}
}

func TestValidateCategoryRejectsUnknownAndDisabled(t *testing.T) {
	value := &Taxonomy{Version: 1, Categories: []Category{
		{Name: "技术", Enabled: true},
		{Name: "停用分类", Enabled: false},
	}}
	if canonical, ok := value.ValidateCategory("技术"); !ok || canonical != "技术" {
		t.Fatalf("expected enabled category, got %q, %v", canonical, ok)
	}
	if _, ok := value.ValidateCategory("停用分类"); ok {
		t.Fatal("disabled category should be rejected")
	}
	if _, ok := value.ValidateCategory("新分类"); ok {
		t.Fatal("unknown category should be rejected")
	}
}

func TestNormalizeMigratesLegacyCategoryAISelection(t *testing.T) {
	value := &Taxonomy{Version: 1, Categories: []Category{
		{Name: "技术", Enabled: true},
		{Name: "停用分类", Enabled: false},
	}}
	normalize(value)
	if value.Version != 2 {
		t.Fatalf("expected taxonomy version 2, got %d", value.Version)
	}
	for _, category := range value.Categories {
		if category.Name == "技术" && !category.AISelectable {
			t.Fatal("legacy enabled category should become AI selectable")
		}
		if category.Name == "停用分类" && category.AISelectable {
			t.Fatal("disabled category must not be AI selectable")
		}
	}
}

func TestAllowedAICategoriesRequiresBothFlags(t *testing.T) {
	value := &Taxonomy{Version: 2, Categories: []Category{
		{Name: "AI 可用", Enabled: true, AISelectable: true},
		{Name: "仅手动", Enabled: true, AISelectable: false},
		{Name: "已停用", Enabled: false, AISelectable: true},
	}}
	allowed := value.AllowedAICategories()
	if len(allowed) != 1 || allowed[0].Name != "AI 可用" {
		t.Fatalf("unexpected AI categories: %#v", allowed)
	}
}

func TestNormalizeDisablesAIForDisabledItems(t *testing.T) {
	value := &Taxonomy{Version: 2,
		Categories: []Category{{Name: "停用分类", Enabled: false, AISelectable: true}},
		Tags:       []ManagedTag{{Name: "停用标签", Enabled: false, AISelectable: true}},
	}
	normalize(value)
	if value.Categories[0].AISelectable || value.Tags[0].AISelectable {
		t.Fatal("disabled taxonomy items must not remain AI selectable")
	}
}
