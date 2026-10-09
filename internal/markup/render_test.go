package markup_test

import (
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestRenderDocument(t *testing.T) {
	tests := []struct{ name, source, format, want string }{
		{"block", "# Heading\n\n:::instruction{priority=critical}\nUse **TDD**.\n:::\n", "xml", "# Heading\n\n<instruction priority=\"critical\">\nUse **TDD**.\n</instruction>\n"},
		{"inline", "Use :term[TTL]{abbr=\"time to live\"} now.", "xml", "Use <term abbr=\"time to live\">TTL</term> now."},
		{"nested", ":::outer\n:::inner\nx\n:::\n:::\n", "xml", "<outer>\n<inner>\nx\n</inner>\n</outer>\n"},
		{"nested directives", ":::outer\n:::inner\nx\n:::\n:::\n", "directives", "::::outer\n:::inner\nx\n:::\n::::\n"},
		{"nested directives attributes", "::::outer{a=1}\n:::inner\nUse :term[x]{b=2}.\n:::\n::::\n", "directives", "::::outer{a=\"1\"}\n:::inner\nUse :term[x]{b=\"2\"}.\n:::\n::::\n"},
		{"nested inline", ":outer[Use :inner[**text**]{}]", "xml", "<outer>Use <inner>**text**</inner></outer>"},
		{"inline context", ":outer[:::literal]", "xml", "<outer>:::literal</outer>"},
		{"CRLF", ":::x\r\ntext\r\n:::\r\n", "xml", "<x>\r\ntext\r\n</x>\r\n"},
		{"XML whitespace", ":x[y]{a=\"line\\nnext\\tcolumn\"}", "xml", "<x a=\"line&#xA;next&#x9;column\">y</x>"},
		{"brackets", ":term[a [link](url) and `]`]{#main .one .two}", "xml", "<term id=\"main\" class=\"one two\">a [link](url) and `]`</term>"},
		{"class key then shorthand", ":x[y]{class=a .b}", "xml", "<x class=\"a b\">y</x>"},
		{"class shorthand then key", ":x[y]{.a class=b}", "xml", "<x class=\"a b\">y</x>"},
		{"normalize", "::: note {key = \"val\"}\nx\n:::\nUse :term[x] {}.", "", ":::note{key=\"val\"}\nx\n:::\nUse :term[x]."},
		{"escaped attributes", ":x[y]{msg=\"Use \\\"quotes\\\" & <tags>\"}", "xml", "<x msg=\"Use &#34;quotes&#34; &amp; &lt;tags&gt;\">y</x>"},
		{"code", "```md\n:::x{bad=}\n:x[bad\n```\n\n`:x[y]`\n\n    :::x\n", "xml", "```md\n:::x{bad=}\n:x[bad\n```\n\n`:x[y]`\n\n    :::x\n"},
		{"code inside directive", ":::x\n```\n:::\n```\n:::\n", "xml", "<x>\n```\n:::\n```\n</x>\n"},
		{"escaped", "\\:::x\n\\:x[y]\n", "xml", "\\:::x\n\\:x[y]\n"},
		{"blockquote", "> :::x\n> text\n> :::\n", "xml", "> <x>\n> text\n> </x>\n"},
		{"list", "- :::x\n  text\n  :::\n", "xml", "- <x>\n  text\n  </x>\n"},
		{"empty", ":::x\n:::", "xml", "<x>\n</x>"},
		{"plain markdown directives", ":::note{priority=high}\nUse :term[TTL]{abbr=cache}.\n:::\n", "markdown", "**note priority=\"high\"**\nUse **term abbr=\"cache\":** TTL.\n\n"},
		{"markdown", "# Heading\n\n- **bold**  \n  next\n\n[link]: ./file.md\n", "xml", "# Heading\n\n- **bold**  \n  next\n\n[link]: ./file.md\n"},
		{"leaf inside container", ":::x\n::y[a]\n:::\n", "xml", "<x>\n<y>a</y>\n</x>\n"},
		{"unlabeled leaf", "::body\n", "xml", "<body></body>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := markup.Parse("skill.md", []byte(tt.source))
			if err != nil {
				t.Fatal(err)
			}
			if err := markup.RenderDocument(doc, tt.format); err != nil {
				t.Fatal(err)
			}
			if got := string(markup.Write(doc)); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}
