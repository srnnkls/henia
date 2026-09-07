package artifact

import "testing"

func TestFrontmatterBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, input, body string
		wantErr           bool
	}{
		{"CRLF", "---\r\nname: test\r\n---\r\n\r\n# Body\r\n", "# Body\r\n", false},
		{"empty", "---\n---\n\n# Body\n", "# Body\n", false},
		{"closing fence at EOF", "---\nname: test\n---", "", false},
		{"unclosed", "---\nname: test\n", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			art, err := Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v", err)
			}
			if err == nil && art.Body != tt.body {
				t.Fatalf("body = %q, want %q", art.Body, tt.body)
			}
		})
	}
}
