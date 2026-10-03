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
	Project Tier = "project"
	Global  Tier = "global"
)

func (t Tier) String() string { return string(t) }

func (t Tier) priority() Priority {
	if t == Global {
		return Fallback
	}
	return Normal
}

var ProjectDirs = []string{".claude/skills", ".agents/skills", ".agent/skills", ".codex/skills", ".pi/skills", ".omp/skills", ".github/skills"}

type Skill struct {
	Path string
	Ref  string
	Tier Tier
	Metadata
}

func (s Skill) Name() string { return filepath.Base(filepath.Dir(s.Path)) }

func Discover(root string, globals []string, sources []library.Source) []Skill {
	var skills []Skill
	seen := make(map[string]bool)
	scan := func(dir string, tier Tier, source string) {
		for _, f := range library.Scan(dir) {
			if seen[f.Physical] {
				continue
			}
			metadata, err := Entries(f.Artifact.Frontmatter)
			if err != nil {
				continue
			}
			seen[f.Physical] = true
			skill := Skill{Path: f.Path, Tier: tier, Metadata: metadata}
			if source != "" {
				skill.Ref = source + ":" + skill.Name()
			}
			skills = append(skills, skill)
		}
	}
	for _, source := range sources {
		tier := Global
		if source.Tier == library.Project {
			tier = Project
		}
		scan(source.Skills(), tier, source.Name)
	}
	if root != "" {
		for _, dir := range ProjectDirs {
			scan(filepath.Join(root, dir), Project, "")
		}
	}
	for _, dir := range globals {
		scan(filepath.Clean(dir), Global, "")
	}
	return skills
}

type Row struct {
	Slot  string `json:"slot"`
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

const (
	Invalid    = "invalid"
	Unknown    = "unknown"
	Undeclared = "undeclared"
	Conflict   = "conflict"
	Selected   = "selected"
	Shadowed   = "shadowed"
)

type Owner struct {
	Slot string `json:"slot"`
	Type Type   `json:"type"`
	Path string `json:"path"`
}

type Provider struct {
	Slot       string    `json:"slot"`
	Entry      string    `json:"entry"`
	Skill      string    `json:"skill"`
	Path       string    `json:"path"`
	Tier       Tier      `json:"tier"`
	Priority   Priority  `json:"priority"`
	Explicit   bool      `json:"explicit"`
	Status     string    `json:"status"`
	Value      string    `json:"value,omitempty"`
	Ref        string    `json:"ref,omitempty"`
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

func Evaluate(skills []Skill) *Resolution {
	r := &Resolution{Owners: []Owner{}, Providers: []*Provider{}, Consumers: []Consumer{}, Problems: []Row{}, declared: make(map[string]Owner)}
	types := make(map[string]map[Type]bool)
	for _, s := range skills {
		for _, entry := range s.Declared {
			d, err := ParseDeclaration(entry)
			if err != nil {
				r.Problems = append(r.Problems, Row{Slot: entry, Kind: Invalid, Path: s.Path})
				continue
			}
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
		for _, entry := range s.Provided {
			d, err := ParseDefinition(entry, s.Tier.priority())
			if err != nil {
				r.Problems = append(r.Problems, Row{Slot: entry, Kind: Invalid, Path: s.Path})
				continue
			}
			p := &Provider{Slot: d.Slot, Entry: entry, Skill: s.Name(), Path: s.Path, Ref: s.Ref, Tier: s.Tier, Priority: d.Priority, Explicit: strings.Contains(entry, "@"), Status: Selected}
			if !r.known(d.Slot) {
				p.Status = Unknown
				r.Problems = append(r.Problems, Row{Slot: d.Slot, Kind: Unknown, Path: s.Path})
			} else if err := p.assign(r, s); err != nil {
				p.Status, p.Reason = Invalid, err.Error()
				r.Problems = append(r.Problems, Row{Slot: d.Slot, Kind: Invalid, Path: s.Path})
			}
			r.Providers = append(r.Providers, p)
		}
	}

	for _, s := range skills {
		for _, entry := range s.Applied {
			slot, err := ParseApplication(entry)
			if err != nil {
				r.Problems = append(r.Problems, Row{Slot: entry, Kind: Invalid, Path: s.Path})
				continue
			}
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
		for _, tier := range []Tier{Project, Global} {
			for _, p := range r.Providers {
				if p.Tier == tier && p.Status == Selected && relevant(p.Slot, requested) {
					rows = append(rows, Row{Slot: p.Slot, Kind: string(p.Tier), Path: p.location(), Value: p.Value})
				}
			}
		}
		for _, request := range requested {
			if !r.declares(request) {
				problems = append(problems, Row{Slot: request, Kind: Undeclared, Path: "-"})
			}
		}
	}
	return dedupe(append(rows, problems...))
}

func (r *Resolution) Select(requested []string) *Resolution {
	selected := &Resolution{Owners: []Owner{}, Providers: []*Provider{}, Problems: r.Rows(requested, true), declared: r.declared}
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

func Applied(skills []Skill, skill string) ([]string, bool) {
	var applied []string
	found := false
	for _, s := range skills {
		if s.Name() == skill {
			found = true
			applied = append(applied, s.Applied...)
		}
	}
	return applied, found
}

func (p *Provider) assign(r *Resolution, s Skill) error {
	owner, _ := r.Owner(p.Slot)
	value, given := s.Values[p.Slot]
	switch owner.Type.Value {
	case SkillValue:
		if given {
			return fmt.Errorf("%s takes a skill, not a value", owner.Slot)
		}
		return nil
	case PathValue:
		if !given || value == "" {
			return fmt.Errorf("%s needs a path", owner.Slot)
		}
		path := filepath.Join(filepath.Dir(s.Path), value)
		if _, err := os.Stat(path); err != nil {
			return err
		}
		p.Value = path
		return nil
	}
	if !given || value == "" {
		return fmt.Errorf("%s needs a %s", owner.Slot, owner.Type.Value)
	}
	p.Value = value
	return nil
}

func (p *Provider) location() string {
	if p.Ref != "" {
		return "henia show " + p.Ref
	}
	return p.Path
}

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

func (r Row) Problem() bool {
	return r.Kind != string(Project) && r.Kind != string(Global)
}
