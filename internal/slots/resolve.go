package slots

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/artifact"
)

type Tier string

const (
	Project Tier = "project"
	Global  Tier = "global"
)

func (t Tier) priority() Priority {
	if t == Global {
		return Fallback
	}
	return Normal
}

var ProjectDirs = []string{".claude/skills", ".agents/skills", ".agent/skills", ".codex/skills", ".pi/skills", ".omp/skills", ".github/skills"}

type Skill struct {
	Path     string
	Tier     Tier
	Declared []string
	Provided []string
}

func Discover(root string, globals []string) []Skill {
	var skills []Skill
	seen := make(map[string]bool)
	scan := func(dir string, tier Tier) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name(), "SKILL.md")
			physical, err := filepath.EvalSymlinks(path)
			if err != nil || seen[physical] {
				continue
			}
			if info, err := os.Stat(physical); err != nil || !info.Mode().IsRegular() {
				continue
			}
			data, err := os.ReadFile(physical)
			if err != nil {
				continue
			}
			art, err := artifact.Parse(data)
			if err != nil {
				continue
			}
			declared, provided, err := Entries(art.Frontmatter)
			if err != nil {
				continue
			}
			seen[physical] = true
			skills = append(skills, Skill{Path: path, Tier: tier, Declared: declared, Provided: provided})
		}
	}
	if root != "" {
		for _, dir := range ProjectDirs {
			scan(filepath.Join(root, dir), Project)
		}
	}
	for _, dir := range globals {
		scan(filepath.Clean(dir), Global)
	}
	return skills
}

type Row struct {
	Slot string `json:"slot"`
	Kind string `json:"kind"`
	Path string `json:"path"`
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
	ShadowedBy *Provider `json:"shadowed_by,omitempty"`
}

type Resolution struct {
	Owners    []Owner     `json:"declarations"`
	Providers []*Provider `json:"providers"`
	Problems  []Row       `json:"problems"`
	declared  map[string]Owner
}

func Evaluate(skills []Skill) *Resolution {
	r := &Resolution{Owners: []Owner{}, Providers: []*Provider{}, Problems: []Row{}, declared: make(map[string]Owner)}
	types := make(map[string]map[Type]bool)
	for _, s := range skills {
		for _, entry := range s.Declared {
			d, err := ParseDeclaration(entry)
			if err != nil {
				r.Problems = append(r.Problems, Row{entry, Invalid, s.Path})
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
			r.Problems = append(r.Problems, Row{owner.Slot, Conflict, owner.Path})
		}
	}

	for _, s := range skills {
		for _, entry := range s.Provided {
			d, err := ParseDefinition(entry, s.Tier.priority())
			if err != nil {
				r.Problems = append(r.Problems, Row{entry, Invalid, s.Path})
				continue
			}
			p := &Provider{Slot: d.Slot, Entry: entry, Skill: filepath.Base(filepath.Dir(s.Path)), Path: s.Path, Tier: s.Tier, Priority: d.Priority, Explicit: strings.Contains(entry, "@"), Status: Selected}
			if !r.known(d.Slot) {
				p.Status = Unknown
				r.Problems = append(r.Problems, Row{d.Slot, Unknown, s.Path})
			}
			r.Providers = append(r.Providers, p)
		}
	}

	for _, p := range r.Providers {
		if p.Status == Unknown {
			continue
		}
		for _, e := range r.Providers {
			if e.Status != Unknown && e.Priority < p.Priority && Within(p.Slot, e.Slot) && (p.ShadowedBy == nil || e.Priority < p.ShadowedBy.Priority) {
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
			r.Problems = append(r.Problems, Row{slot, Conflict, "-"})
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
					rows = append(rows, Row{p.Slot, string(p.Tier), p.Path})
				}
			}
		}
		for _, request := range requested {
			if !r.declares(request) {
				problems = append(problems, Row{request, Undeclared, "-"})
			}
		}
	}
	return dedupe(append(rows, problems...))
}

func (r *Resolution) Select(requested []string) *Resolution {
	selected := &Resolution{Owners: []Owner{}, Providers: []*Provider{}, Problems: r.Rows(requested, true), declared: r.declared}
	for _, request := range requested {
		if !r.declares(request) {
			selected.Problems = append(selected.Problems, Row{request, Undeclared, "-"})
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
	return selected
}

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
