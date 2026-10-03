package slots

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

func (r *Resolution) Explain(w io.Writer, requested []string) error {
	var b strings.Builder
	if len(requested) == 0 {
		requested = r.roots()
	}
	seen := make(map[string]bool)
	for _, request := range requested {
		if seen[request] {
			continue
		}
		seen[request] = true
		fmt.Fprintln(&b, request)
		if !r.declares(request) {
			fmt.Fprintln(&b, "  undeclared: no skill declares this slot or an enclosing one")
		}
		var slots []string
		for _, owner := range r.Owners {
			if related(owner.Slot, request) && !slices.Contains(slots, owner.Slot) {
				slots = append(slots, owner.Slot)
			}
		}
		for _, p := range r.Providers {
			if related(p.Slot, request) && !slices.Contains(slots, p.Slot) {
				slots = append(slots, p.Slot)
			}
		}
		for _, c := range r.Consumers {
			if related(c.Slot, request) && !slices.Contains(slots, c.Slot) {
				slots = append(slots, c.Slot)
			}
		}
		slices.Sort(slots)
		for _, slot := range slots {
			fmt.Fprintf(&b, "  %s %s\n", slot, r.typing(slot))
			var consumers []string
			for _, c := range r.Consumers {
				if c.Slot == slot && !slices.Contains(consumers, c.Skill) {
					consumers = append(consumers, c.Skill)
				}
			}
			if len(consumers) > 0 {
				fmt.Fprintf(&b, "    applied by %s\n", strings.Join(consumers, ", "))
			}
			selected, listed := 0, 0
			for _, p := range r.Providers {
				if p.Slot != slot {
					continue
				}
				listed++
				fmt.Fprintf(&b, "    %-8s %s [%s] %s\n", p.Status, p.Skill, p.origin(), p.Path)
				switch p.Status {
				case Shadowed:
					fmt.Fprintf(&b, "      by %s [%s] for %s, %s\n", p.ShadowedBy.Skill, p.ShadowedBy.origin(), p.ShadowedBy.Slot, p.ShadowedBy.Path)
				case Unknown:
					fmt.Fprintf(&b, "      %s\n", r.unknownReason(slot))
				case Selected:
					selected++
				}
			}
			if listed == 0 {
				fmt.Fprintln(&b, "    no providers")
			}
			if owner, _ := r.Owner(slot); owner.Type.Unique && selected > 1 {
				fmt.Fprintf(&b, "    conflict: %s admits one provider, %d are selected\n", owner.Type, selected)
			}
		}
	}
	for _, problem := range r.Problems {
		if problem.Kind == Invalid || problem.Kind == Conflict && problem.Path != "-" {
			fmt.Fprintf(&b, "%s %s %s\n", problem.Kind, problem.Slot, problem.Path)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func (r *Resolution) roots() []string {
	var roots []string
	add := func(slot string) {
		root, _, _ := strings.Cut(slot, ".")
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	for _, owner := range r.Owners {
		add(owner.Slot)
	}
	for _, p := range r.Providers {
		add(p.Slot)
	}
	for _, c := range r.Consumers {
		add(c.Slot)
	}
	slices.Sort(roots)
	return roots
}

func (p *Provider) origin() string {
	source := "tier"
	if p.Explicit {
		source = "entry"
	}
	return fmt.Sprintf("%s, %s from %s", p.Tier, p.Priority, source)
}

func (r *Resolution) typing(slot string) string {
	owner, ok := r.Owner(slot)
	switch {
	case !ok:
		return "(undeclared)"
	case owner.Path == "":
		return "(" + owner.Type.String() + ", untyped: no declarations installed)"
	case owner.Slot == slot:
		return "(" + owner.Type.String() + ", declared by " + owner.Path + ")"
	}
	return "(" + owner.Type.String() + " of " + owner.Slot + ", declared by " + owner.Path + ")"
}

func (r *Resolution) unknownReason(slot string) string {
	if owner, ok := r.Owner(slot); ok {
		return owner.Slot + " is " + owner.Type.String() + ", not keyed"
	}
	return "no declaration covers this slot"
}
