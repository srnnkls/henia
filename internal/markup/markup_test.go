package markup_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestRender(t *testing.T) {
	tests := []struct{ name, source, format, want string }{
		{"block", "# Heading\n\n:::instruction{priority=critical}\nUse **TDD**.\n:::\n", "xml", "# Heading\n\n<instruction priority=\"critical\">\nUse **TDD**.\n</instruction>\n"},
		{"inline", "Use :term[TTL]{abbr=\"time to live\"} now.", "xml", "Use <term abbr=\"time to live\">TTL</term> now."},
		{"nested", ":::outer\n:::inner\nx\n:::\n:::\n", "xml", "<outer>\n<inner>\nx\n</inner>\n</outer>\n"},
		{"nested inline", ":outer[Use :inner[**text**]{}]", "xml", "<outer>Use <inner>**text**</inner></outer>"},
		{"inline context", ":outer[:::literal]", "xml", "<outer>:::literal</outer>"},
		{"CRLF", ":::x\r\ntext\r\n:::\r\n", "xml", "<x>\r\ntext\r\n</x>\r\n"},
		{"XML whitespace", ":x[y]{a=\"line\\nnext\\tcolumn\"}", "xml", "<x a=\"line&#xA;next&#x9;column\">y</x>"},
		{"brackets", ":term[a [link](url) and `]`]{#main .one .two}", "xml", "<term id=\"main\" class=\"one two\">a [link](url) and `]`</term>"},
		{"normalize", "::: note {key = \"val\"}\nx\n:::\nUse :term[x] {}.", "", ":::note{key=\"val\"}\nx\n:::\nUse :term[x]."},
		{"escaped attributes", ":x[y]{msg=\"Use \\\"quotes\\\" & <tags>\"}", "xml", "<x msg=\"Use &#34;quotes&#34; &amp; &lt;tags&gt;\">y</x>"},
		{"code", "```md\n:::x{bad=}\n:x[bad\n```\n\n`:x[y]`\n\n    :::x\n", "xml", "```md\n:::x{bad=}\n:x[bad\n```\n\n`:x[y]`\n\n    :::x\n"},
		{"code inside directive", ":::x\n```\n:::\n```\n:::\n", "xml", "<x>\n```\n:::\n```\n</x>\n"},
		{"escaped", "\\:::x\n\\:x[y]\n", "xml", "\\:::x\n\\:x[y]\n"},
		{"blockquote", "> :::x\n> text\n> :::\n", "xml", "> <x>\n> text\n> </x>\n"},
		{"list", "- :::x\n  text\n  :::\n", "xml", "- <x>\n  text\n  </x>\n"},
		{"empty", ":::x\n:::", "xml", "<x>\n</x>"},
		{"markdown", "# Heading\n\n- **bold**  \n  next\n\n[link]: ./file.md\n", "xml", "# Heading\n\n- **bold**  \n  next\n\n[link]: ./file.md\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := markup.Render(tt.source, tt.format)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestInvalidMarkup(t *testing.T) {
	for _, input := range []string{":::x{attr=}\nx\n:::", ":x[y]{key=value with spaces}", ":::x\ntext", ":x[unclosed", ":::x{a=1 a=2}\n:::", ":x[y]{a=[1,2]}", ":outer[:inner[x]{a=}]", ":x[y]{a=\"\\b\"}"} {
		t.Run(input, func(t *testing.T) {
			_, err := markup.Render("intro\n\n"+input, "xml")
			var location *markup.Error
			if !errors.As(err, &location) || location.Line != 3 || location.Column < 1 {
				t.Fatalf("expected line 3 diagnostic, got %v", err)
			}
		})
	}
}

func BenchmarkRender(b *testing.B) {
	source := strings.Repeat(":::instruction{priority=critical}\nUse :term[TTL]{abbr=\"time to live\"}.\n:::\n\n", 140)
	for b.Loop() {
		if _, err := markup.Render(source, "xml"); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzRender(f *testing.F) {
	for _, seed := range []string{":::x\n:x[y]{a=1}\n:::", ":x[`]`]{a=}", "", "\\:x[y]"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		normalized, err := markup.Render(source, "directives")
		if err != nil {
			return
		}
		again, err := markup.Render(normalized, "directives")
		if err != nil || again != normalized {
			t.Fatalf("normalization not idempotent: %q -> %q (%v)", normalized, again, err)
		}
	})
}
