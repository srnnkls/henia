package pattern

import (
	"errors"
	"testing"
)

func TestReadTopLevelRepeatHint(t *testing.T) {
	for _, query := range []string{`(code)+ @c`, `(code)* @c`} {
		_, err := Read(query)
		var pe *Error
		if !errors.As(err, &pe) {
			t.Fatalf("Read(%q) error = %v, want *Error", query, err)
		}
		if pe.Hint == "" {
			t.Errorf("Read(%q) hint is empty, want a hint for top-level %s", query, query[6:7])
		}
	}
}
