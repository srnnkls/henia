// Package units installs config files that the harness also writes. Henia
// owns units of such a file: top-level values, or the members of declared
// entry paths. Values it does not own are never read or changed.
package units

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// File is one destination. A nil Value removes every unit a previous install
// recorded for it.
type File struct {
	Path     string
	Format   string // json or toml
	Entries  []string
	Value    map[string]any
	Validate func(map[string]any) error
	Strict   bool // A validation error fails the apply instead of warning.
}

// Record maps each destination to the units Henia wrote there and their hashes.
type Record map[string]map[string]string

// Conflict lists units that hold values Henia did not write.
type Conflict struct {
	Path  string
	Units []string
}

func (c *Conflict) Error() string {
	return fmt.Sprintf("%s: %s not installed by henia or edited since", c.Path, strings.Join(c.Units, ", "))
}

// Apply merges every file's units into its destination and returns the new
// record. Without force, a unit holding a value Henia did not write fails the
// whole apply before anything is written.
func Apply(files []File, previous Record, force bool) (Record, []string, error) {
	return run(files, previous, force, true)
}

// Check reports the conflicts and validation failures Apply would hit,
// without writing.
func Check(files []File, previous Record) error {
	_, _, err := run(files, previous, false, false)
	return err
}

func run(files []File, previous Record, force, commit bool) (Record, []string, error) {
	byPath := map[string]File{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	for path := range previous {
		if _, ok := byPath[path]; !ok {
			byPath[path] = File{Path: path, Format: formatOf(path)}
		}
	}
	type plan struct {
		file     File
		original []byte
		data     []byte
		units    map[string]string
	}
	var plans []plan
	var conflicts []error
	var warnings []string
	for _, path := range slices.Sorted(maps.Keys(byPath)) {
		file := byPath[path]
		original, err := read(path)
		if err != nil {
			return nil, nil, err
		}
		data, units, conflict, err := merge(file, original, previous[path], force)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		if conflict != nil {
			conflicts = append(conflicts, conflict)
			continue
		}
		if file.Validate != nil && data != nil {
			doc, err := Decode(file.Format, data)
			if err == nil {
				err = file.Validate(doc)
			}
			if err != nil && file.Strict {
				return nil, nil, fmt.Errorf("%s: %w", path, err)
			}
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			}
		}
		plans = append(plans, plan{file, original, data, units})
	}
	if len(conflicts) > 0 {
		return nil, nil, errors.Join(conflicts...)
	}
	record := Record{}
	if !commit {
		return record, warnings, nil
	}
	for _, p := range plans {
		if len(p.units) > 0 {
			record[p.file.Path] = p.units
		}
		if p.data == nil || bytes.Equal(p.data, p.original) {
			continue
		}
		if err := write(p.file.Path, p.original, p.data); err != nil {
			return nil, nil, err
		}
	}
	return record, warnings, nil
}

func formatOf(path string) string {
	if strings.HasSuffix(path, ".toml") {
		return "toml"
	}
	return "json"
}

func read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// merge returns the destination's new content, or nil when it stays as is.
func merge(file File, original []byte, previous map[string]string, force bool) ([]byte, map[string]string, *Conflict, error) {
	doc := map[string]any{}
	if len(bytes.TrimSpace(original)) > 0 {
		var err error
		if doc, err = Decode(file.Format, original); err != nil {
			return nil, nil, nil, err
		}
	}
	var conflicted []string
	if file.Format == "toml" && hasComments(original) && file.Value != nil {
		conflicted = append(conflicted, "comments")
	}
	units := map[string]string{}
	wanted := flatten(file.Value, file.Entries)
	for _, u := range wanted {
		id, h := u.id(), hash(u.value)
		units[id] = h
		if u.elem {
			items, exists, ok := array(doc, u.path)
			if !ok {
				conflicted = append(conflicted, u.name())
				if force {
					set(doc, u.path, []any{u.value})
				}
				continue
			}
			if !exists || !containsHash(items, h) {
				set(doc, u.path, append(items, u.value))
			}
			continue
		}
		current, exists := get(doc, u.path)
		switch {
		case !exists, previous[id] == hash(current):
			if err := set(doc, u.path, u.value); err != nil {
				conflicted = append(conflicted, u.name())
			}
		case hash(current) == h:
		default:
			conflicted = append(conflicted, u.name())
			if force {
				set(doc, u.path, u.value)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(previous)) {
		if _, kept := units[id]; kept {
			continue
		}
		u, err := parseID(id)
		if err != nil {
			return nil, nil, nil, err
		}
		if u.elem {
			if items, exists, ok := array(doc, u.path); exists && ok {
				set(doc, u.path, slices.DeleteFunc(items, func(item any) bool { return hash(item) == previous[id] }))
			}
			continue
		}
		current, exists := get(doc, u.path)
		switch {
		case !exists:
		case hash(current) == previous[id], force:
			del(doc, u.path)
		default:
			conflicted = append(conflicted, u.name())
		}
	}
	if len(conflicted) > 0 && !force {
		return nil, nil, &Conflict{file.Path, conflicted}, nil
	}
	if file.Value == nil && len(original) == 0 {
		return nil, units, nil, nil
	}
	data, err := encode(file.Format, doc)
	if err != nil {
		return nil, nil, nil, err
	}
	if bytes.Equal(canonical(file.Format, original), canonical(file.Format, data)) {
		return nil, units, nil, nil
	}
	return data, units, nil, nil
}

func canonical(format string, data []byte) []byte {
	doc, err := Decode(format, data)
	if err != nil {
		return data
	}
	out, _ := json.Marshal(doc)
	return out
}

func hasComments(data []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if strings.HasPrefix(strings.TrimSpace(scanner.Text()), "#") {
			return true
		}
	}
	return false
}

func Decode(format string, data []byte) (map[string]any, error) {
	doc := map[string]any{}
	if format == "toml" {
		return doc, toml.Unmarshal(data, &doc)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func encode(format string, doc map[string]any) ([]byte, error) {
	if format == "toml" {
		return toml.Marshal(doc)
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(doc)
	return buf.Bytes(), err
}

// write replaces the destination atomically unless it changed since it was read.
func write(path string, original, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".henia-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporary.Name(), mode); err != nil {
		return err
	}
	current, err := read(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("%s changed while installing; run the install again", path)
	}
	return os.Rename(temporary.Name(), path)
}

type unit struct {
	path  []string
	elem  bool
	value any
}

func (u unit) id() string {
	data, _ := json.Marshal(struct {
		Path []string `json:"path"`
		Elem bool     `json:"elem,omitempty"`
	}{u.path, u.elem})
	if u.elem {
		return string(data) + "#" + hash(u.value)
	}
	return string(data)
}

func (u unit) name() string {
	name := strings.Join(u.path, ".")
	if u.elem {
		data, _ := json.Marshal(u.value)
		return name + "[" + string(data) + "]"
	}
	return name
}

func parseID(id string) (unit, error) {
	head, _, _ := strings.Cut(id, "#")
	var parsed struct {
		Path []string `json:"path"`
		Elem bool     `json:"elem"`
	}
	if err := json.Unmarshal([]byte(head), &parsed); err != nil {
		return unit{}, fmt.Errorf("unit %q: %w", id, err)
	}
	return unit{path: parsed.Path, elem: parsed.Elem}, nil
}

func flatten(value map[string]any, entries []string) []unit {
	declared, prefixes := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		declared[entry] = true
		parts := strings.Split(entry, ".")
		for i := 1; i < len(parts); i++ {
			prefixes[strings.Join(parts[:i], ".")] = true
		}
	}
	var units []unit
	var walk func(object map[string]any, path []string)
	walk = func(object map[string]any, path []string) {
		for _, key := range slices.Sorted(maps.Keys(object)) {
			child, name := append(slices.Clone(path), key), strings.Join(append(slices.Clone(path), key), ".")
			switch value := object[key].(type) {
			case map[string]any:
				if declared[name] {
					for _, member := range slices.Sorted(maps.Keys(value)) {
						units = append(units, unit{path: append(slices.Clone(child), member), value: value[member]})
					}
					continue
				}
				if prefixes[name] {
					walk(value, child)
					continue
				}
			case []any:
				if declared[name] {
					for _, item := range value {
						units = append(units, unit{path: child, elem: true, value: item})
					}
					continue
				}
			}
			units = append(units, unit{path: child, value: object[key]})
		}
	}
	walk(value, nil)
	return units
}

func hash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func containsHash(items []any, h string) bool {
	return slices.ContainsFunc(items, func(item any) bool { return hash(item) == h })
}

func get(doc map[string]any, path []string) (any, bool) {
	var current any = doc
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = object[key]; !ok {
			return nil, false
		}
	}
	return current, true
}

func array(doc map[string]any, path []string) ([]any, bool, bool) {
	value, exists := get(doc, path)
	if !exists {
		return nil, false, true
	}
	items, ok := value.([]any)
	return items, true, ok
}

func set(doc map[string]any, path []string, value any) error {
	object := doc
	for _, key := range path[:len(path)-1] {
		next, exists := object[key]
		if !exists {
			created := map[string]any{}
			object[key], object = created, created
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("%s is not a table", key)
		}
		object = child
	}
	object[path[len(path)-1]] = value
	return nil
}

func del(doc map[string]any, path []string) {
	object := doc
	for _, key := range path[:len(path)-1] {
		child, ok := object[key].(map[string]any)
		if !ok {
			return
		}
		object = child
	}
	delete(object, path[len(path)-1])
}
