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
