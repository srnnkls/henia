package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/preload"
	"github.com/srnnkls/henia/internal/slots"
)

const inlineDepth = 4

var (
	slotsPreload   = regexp.MustCompile(`^henia slots --(markdown|inline)((?: [A-Za-z0-9_.-]+)+)$`)
	tocPreload     = regexp.MustCompile(`^henia show (\S+) --toc$`)
	contextPreload = regexp.MustCompile(`^henia context (\S+)$`)
)

type resolver struct {
	lib        *library.Library
	render     *renderer
	project    string
	resolution *slots.Resolution
	err        error
	depth      int
}

func newResolver(lib *library.Library, r *renderer, project string) *resolver {
	return &resolver{lib: lib, render: r, project: project}
}

func (x *resolver) resolve(command string) (string, bool) {
	if m := slotsPreload.FindStringSubmatch(command); m != nil {
		return x.slots(m[1] == "inline", strings.Fields(m[2])), true
	}
	if m := tocPreload.FindStringSubmatch(command); m != nil {
		entry, err := x.lib.Resolve(m[1])
		if err != nil {
			return fmt.Sprintf("henia: %v\n", err), true
		}
		var b strings.Builder
		contents(&b, x.lib.Reference(entry), x.render.body(entry))
		return b.String(), true
	}
	if m := contextPreload.FindStringSubmatch(command); m != nil {
		entry, err := x.lib.Resolve(m[1])
		if err != nil {
			return fmt.Sprintf("henia: %v\n", err), true
		}
		var b strings.Builder
		related(&b, x.lib, x.render, entry)
		return strings.TrimLeft(b.String(), "\n"), true
	}
	return "", false
}

func (x *resolver) slots(inline bool, requested []string) string {
	if x.resolution == nil && x.err == nil {
		x.resolution, x.err = resolveSlots(x.project, x.lib)
	}
	if x.err != nil {
		return fmt.Sprintf("henia: slots not resolved: %v\n", x.err)
	}
	if !inline || x.depth >= inlineDepth {
		return x.resolution.Markdown(requested)
	}
	return x.resolution.Inline(requested, x.content)
}

func (x *resolver) content(p *slots.Provider, kind slots.Value) string {
	switch kind {
	case slots.CommandValue:
		return "```bash\n" + p.Value + "\n```"
	case slots.TextValue:
		return p.Value
	case slots.PathValue:
		text, err := library.ReadResource(p.Value)
		if err != nil {
			return fmt.Sprintf("henia: %s: %v", p.Slot, err)
		}
		return x.nested(text)
	}
	entry, err := x.lib.Resolve(p.Ref)
	if err != nil {
		return fmt.Sprintf("henia: %s: %v", p.Slot, err)
	}
	body := x.render.body(entry)
	if p.Section != "" {
		_, text, ok := library.Section(body, p.Section)
		if !ok {
			return fmt.Sprintf("henia: %s: %s has no section %q", p.Slot, p.Ref, p.Section)
		}
		body = text
	}
	return x.nested(body)
}

func (x *resolver) nested(text string) string {
	x.depth++
	defer func() { x.depth-- }()
	return preload.Substitute(text, x.resolve)
}
