package sitepages

import "testing"

func TestValidateRejectsDuplicateKeys(t *testing.T) {
	value := &Pages{Version: 1, Pages: []Page{
		{Key: "home", Name: "首页", Title: "首页", Heading: "文章"},
		{Key: "home", Name: "重复", Title: "重复", Heading: "重复"},
	}}
	if err := Validate(value); err == nil {
		t.Fatal("expected duplicate page key to be rejected")
	}
}
