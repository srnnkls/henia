package canonical

import (
	"reflect"
	"testing"
)

func TestRoundTripKeepsFalseEmptyAndExtra(t *testing.T) {
	input := map[string]any{"name": "example", "description": "Example", "enabled": false, "user_invocable": false, "model_tier": "strong", "tools": []string{}, "tools_policy": map[string]any{"write": "deny"}, "custom": "extra"}
	c, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.ToMap(), input) {
		t.Fatalf("%+v", c.ToMap())
	}
}

func TestCanonicalRejectsIncorrectTypes(t *testing.T) {
	for key, value := range map[string]any{"name": 42, "enabled": "false", "tools": "read", "tools_policy": true, "user_invocable": "false"} {
		input := map[string]any{"name": "example", key: value}
		if _, err := Parse(input); err == nil {
			t.Fatalf("accepted %s: %v", key, value)
		}
	}
}
