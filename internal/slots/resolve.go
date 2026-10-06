package slots

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/library"
)

type Tier string

const (
	Project    Tier = library.Project
	Dependency Tier = library.Dependency
	Global     Tier = library.Global
)

var tiers = []Tier{Project, Dependency, Global}

func (t Tier) String() string { return string(t) }

func (t Tier) priority() Priority {
	switch t {
	case Global:
		return Fallback
	case Dependency:
		return inherited
	}
	return Normal
}

type Skill struct {
	Path string
	Ref  string
	Tier Tier
	Metadata
}

func (s Skill) Name() string { return filepath.Base(filepath.Dir(s.Path)) }

func Discover(lib *library.Library) []Skill {
	var skills []Skill
	for _, e := range lib.Entries {
		tier := Tier(e.Tier)
		skills = append(skills, Skill{Path: e.Path, Ref: e.ID, Tier: tier, Metadata: Read(e.Artifact.Frontmatter, e.Artifact.Body, tier.priority())})
	}
	return skills
}

type Row struct {
	Slot     string    `json:"slot"`
	Kind     string    `json:"kind"`
	Path     string    `json:"path"`
	Value    string    `json:"value,omitempty"`
	provider *Provider `json:"-"`
}

const (
	Invalid    = "invalid"
	Unknown    = "unknown"
	Undeclared = "undeclared"
	Conflict   = "conflict"
	Selected   = "selected"
	Shadowed   = "shadowed"
	Disabled   = "disabled"
	None       = "none"
)

type Owner struct {
	Slot string `json:"slot"`
	Type Type   `json:"type"`
	Path string `json:"path"`
}

type Provider struct {
	Slot       string    `json:"slot"`
	Skill      string    `json:"skill"`
	Path       string    `json:"path"`
	Tier       Tier      `json:"tier"`
	Priority   Priority  `json:"priority"`
	Explicit   bool      `json:"explicit"`
	Status     string    `json:"status"`
	Value      string    `json:"value,omitempty"`
	Section    string    `json:"section,omitempty"`
	Ref        string    `json:"ref"`
	Reason     string    `json:"reason,omitempty"`
	ShadowedBy *Provider `json:"shadowed_by,omitempty"`
}

type Consumer struct {
	Slot  string `json:"slot"`
	Skill string `json:"skill"`
	Path  string `json:"path"`
	Tier  Tier   `json:"tier"`
}

type Resolution struct {
	Owners    []Owner     `json:"declarations"`
	Providers []*Provider `json:"providers"`
	Consumers []Consumer  `json:"consumers"`
	Problems  []Row       `json:"problems"`
	declared  map[string]Owner
}

func Evaluate(skills []Skill, disabled map[string][]string) *Resolution {
	r := &Resolution{Owners: []Owner{}, Providers: []*Provider{}, Consumers: []Consumer{}, Problems: []Row{}, declared: make(map[string]Owner)}
	types := make(map[string]map[Type]bool)
	for _, s := range skills {
		for _, problem := range s.Problems {
			r.Problems = append(r.Problems, Row{Slot: problem.Slot, Kind: Invalid, Path: s.Path})
		}
		for _, d := range s.Declared {
			owner := Owner{d.Slot, d.Type, s.Path}
			r.Owners = append(r.Owners, owner)
			if _, ok := r.declared[d.Slot]; !ok {
				r.declared[d.Slot] = owner
				types[d.Slot] = make(map[Type]bool)
			}
			types[d.Slot][d.Type] = true
		}
	}
	for _, owner := range r.Owners {
		if len(types[owner.Slot]) > 1 {
			r.Problems = append(r.Problems, Row{Slot: owner.Slot, Kind: Conflict, Path: owner.Path})
		}
	}

	for _, s := range skills {
		for _, o := range s.Offered {
			p := &Provider{Slot: o.Slot, Skill: s.Name(), Path: s.Path, Ref: s.Ref, Tier: s.Tier, Priority: o.Priority, Explicit: o.Explicit, Section: o.Section, Status: Selected}
			switch {
			case !r.known(o.Slot):
				p.Status = Unknown
				r.Problems = append(r.Problems, Row{Slot: o.Slot, Kind: Unknown, Path: s.Path})
			case off(disabled, o.Slot, s.Ref):
				p.Status = Disabled
			default:
				if err := p.assign(r, s, o); err != nil {
					p.Status, p.Reason = Invalid, err.Error()
					r.Problems = append(r.Problems, Row{Slot: o.Slot, Kind: Invalid, Path: s.Path})
				}
			}
			r.Providers = append(r.Providers, p)
		}
	}

	for _, s := range skills {
		for _, slot := range s.Applied {
			r.Consumers = append(r.Consumers, Consumer{slot, s.Name(), s.Path, s.Tier})
			if !r.declares(slot) {
				r.Problems = append(r.Problems, Row{Slot: slot, Kind: Undeclared, Path: s.Path})
			}
		}
	}

	for _, p := range r.Providers {
		if !p.competes() {
			continue
		}
		for _, e := range r.Providers {
			if e.competes() && e.Priority < p.Priority && Within(p.Slot, e.Slot) && (p.ShadowedBy == nil || e.Priority < p.ShadowedBy.Priority) {
				p.Status, p.ShadowedBy = Shadowed, e
			}
		}
	}

	unique := make(map[string]map[string]bool)
	for _, p := range r.Providers {
		if owner, _ := r.Owner(p.Slot); p.Status == Selected && owner.Type.Unique {
			if unique[p.Slot] == nil {
				unique[p.Slot] = make(map[string]bool)
			}
			unique[p.Slot][p.Path] = true
		}
	}
	for _, slot := range slices.Sorted(maps.Keys(unique)) {
		if len(unique[slot]) > 1 {
			r.Problems = append(r.Problems, Row{Slot: slot, Kind: Conflict, Path: "-"})
		}
	}
	return r
}

func (r *Resolution) Owner(slot string) (Owner, bool) {
	if len(r.declared) == 0 {
		return Owner{Slot: slot, Type: Untyped}, true
	}
	for key := slot; ; {
		if owner, ok := r.declared[key]; ok {
			return owner, true
		}
		i := strings.LastIndexByte(key, '.')
		if i < 0 {
			return Owner{}, false
		}
		key = key[:i]
	}
}

func (r *Resolution) known(slot string) bool {
	owner, ok := r.Owner(slot)
	return ok && (owner.Slot == slot || owner.Type.Keyed)
}

func (r *Resolution) declares(request string) bool {
	return len(r.declared) == 0 || slices.ContainsFunc(slices.Collect(maps.Keys(r.declared)), func(slot string) bool { return related(request, slot) })
}

func (r *Resolution) Rows(requested []string, check bool) []Row {
	var rows []Row
	problems := slices.Clone(r.Problems)
	if !check {
		for _, tier := range tiers {
			for _, p := range r.Providers {
				if p.Tier == tier && p.Status == Selected && relevant(p.Slot, requested) {
					rows = append(rows, Row{Slot: p.Slot, Kind: string(p.Tier), Path: p.location(), Value: p.Value, provider: p})
				}
			}
		}
		problems = slices.DeleteFunc(problems, func(row Row) bool { return !relevant(row.Slot, requested) })
		for _, request := range requested {
			if !r.declares(request) {
				problems = append(problems, Row{Slot: request, Kind: Undeclared, Path: "-"})
			} else if !slices.ContainsFunc(rows, func(row Row) bool { return related(row.Slot, request) }) {
				rows = append(rows, Row{Slot: request, Kind: None, Path: "-"})
			}
		}
	}
	return dedupe(append(rows, problems...))
}

func (r *Resolution) Select(requested []string) *Resolution {
	selected := &Resolution{Owners: []Owner{}, Providers: []*Provider{}, Consumers: []Consumer{}, Problems: r.Rows(requested, true), declared: r.declared}
	for _, request := range requested {
		if !r.declares(request) {
			selected.Problems = append(selected.Problems, Row{Slot: request, Kind: Undeclared, Path: "-"})
		}
	}
	for _, owner := range r.Owners {
		if len(requested) == 0 || relevant(owner.Slot, requested) {
			selected.Owners = append(selected.Owners, owner)
		}
	}
	for _, p := range r.Providers {
		if len(requested) == 0 || relevant(p.Slot, requested) {
			selected.Providers = append(selected.Providers, p)
		}
	}
	for _, c := range r.Consumers {
		if len(requested) == 0 || relevant(c.Slot, requested) {
			selected.Consumers = append(selected.Consumers, c)
		}
	}
	return selected
}

func off(disabled map[string][]string, slot, ref string) bool {
	for key, refs := range disabled {
		if Within(slot, key) && slices.Contains(refs, ref) {
			return true
		}
	}
	return false
}

func (p *Provider) assign(r *Resolution, s Skill, o Offer) error {
	owner, _ := r.Owner(p.Slot)
	want := owner.Type.Value
	switch {
	case want == SkillValue && o.Kind != SkillValue:
		return fmt.Errorf("%s takes a skill, not a %s", owner.Slot, o.Kind)
	case want != SkillValue && o.Kind != want:
		return fmt.Errorf("%s needs a %s", owner.Slot, want)
	case want == PathValue:
		path := filepath.Join(filepath.Dir(s.Path), o.Value)
		if _, err := os.Stat(path); err != nil {
			return err
		}
		p.Value = path
	default:
		p.Value = o.Value
	}
	return nil
}

func (p *Provider) address() string {
	if p.Section != "" {
		return p.Ref + "#" + p.Section
	}
	return p.Ref
}

func (p *Provider) location() string { return library.ShowCommand(p.address()) }

func (p *Provider) competes() bool { return p.Status == Selected || p.Status == Shadowed }

func relevant(slot string, requested []string) bool {
	return slices.ContainsFunc(requested, func(request string) bool { return related(slot, request) })
}

func dedupe(rows []Row) []Row {
	seen := make(map[Row]bool)
	result := []Row{}
	for _, r := range rows {
		if !seen[r] {
			seen[r] = true
			result = append(result, r)
		}
	}
	return result
}

func (r Row) Line() string {
	line := r.Slot + "\t" + r.Kind + "\t" + r.Path
	if r.Value != "" {
		line += "\t" + r.Value
	}
	return line
}

func (r Row) Problem() bool {
	return r.Kind != None && !slices.Contains(tiers, Tier(r.Kind))
}

func (r *Resolution) Inline(requested []string, content func(p *Provider, kind Value) string) string {
	var parts []string
	for _, row := range r.ordered(requested) {
		switch {
		case row.Kind == None:
		case row.Problem():
			parts = append(parts, "henia: slot `"+row.Slot+"`: "+r.pointer(row))
		default:
			owner, _ := r.Owner(row.provider.Slot)
			if text := strings.TrimSpace(content(row.provider, owner.Type.Value)); text != "" {
				parts = append(parts, text)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func (r *Resolution) Markdown(requested []string) string {
	var b strings.Builder
	for _, row := range r.ordered(requested) {
		fmt.Fprintf(&b, "- `%s`: %s\n", row.Slot, r.pointer(row))
	}
	return b.String()
}

func (r *Resolution) pointer(row Row) string {
	switch {
	case row.Kind == None:
		return "no provider"
	case row.Problem():
		if row.Path == "-" {
			return row.Kind
		}
		return row.Kind + " in `" + row.Path + "`"
	}
	p := row.provider
	owner, _ := r.Owner(p.Slot)
	switch owner.Type.Value {
	case CommandValue:
		return "`" + p.Value + "`"
	case TextValue:
		return p.Value
	case PathValue:
		if rel, err := filepath.Rel(filepath.Dir(p.Path), p.Value); err == nil && filepath.IsLocal(rel) {
			return "`" + library.ShowCommand(library.Address(p.Ref, rel)) + "`"
		}
		return "`" + p.Value + "`"
	}
	return "`" + row.Path + "`"
}

func (r *Resolution) ordered(requested []string) []Row {
	rows := r.Rows(requested, false)
	slices.SortStableFunc(rows, func(a, b Row) int {
		return request(a.Slot, requested) - request(b.Slot, requested)
	})
	return rows
}

func request(slot string, requested []string) int {
	if i := slices.IndexFunc(requested, func(request string) bool { return related(slot, request) }); i >= 0 {
		return i
	}
	return len(requested)
}
