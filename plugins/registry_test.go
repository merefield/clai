package plugins

import "testing"

func TestRegistryIncludesWikipediaTutorialPlugin(t *testing.T) {
	registry, err := Registry(nil)
	if err != nil {
		t.Fatal(err)
	}
	names := registry.Names()
	if len(names) != 1 || names[0] != "wikipedia_lookup" {
		t.Fatalf("registered tools = %#v", names)
	}
}
