package cli

import (
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

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/profile"
	"github.com/srnnkls/henia/internal/units"
)

// placements collects the config file placements of every profile the
// harnesses name.
func placements(harnesses ...henia.Harness) (map[string]profile.Placement, error) {
	result := map[string]profile.Placement{}
	for _, h := range harnesses {
		if h.Profile == "" {
			continue
		}
		loaded, err := profile.Load(h.Profile, h.ProjectRoot, h.UserRoot)
		if err != nil {
			return nil, err
		}
		maps.Copy(result, loaded.Config.Placements())
	}
	return result, nil
}

func sibling(manifest, suffix string) string {
	return strings.TrimSuffix(manifest, ".json") + "." + suffix + ".json"
}

// recordSet lists manifest and the records kept beside it for one install.
func recordSet(manifest string) []string {
	return []string{manifest, sibling(manifest, "units"), sibling(manifest, "project"), sibling(manifest, "home")}
}

// parked names where the records of an install into target wait while
// manifest holds another target's.
func parked(manifest, target string) string {
	sum := sha256.Sum256([]byte(target))
	return strings.TrimSuffix(manifest, ".json") + "@" + hex.EncodeToString(sum[:6]) + ".json"
}

// moveRecords moves the record set at from onto to, leaving no record at to
// that from did not hold.
func moveRecords(from, to string) error {
	destinations := recordSet(to)
	for i, source := range recordSet(from) {
		err := os.Rename(source, destinations[i])
		if errors.Is(err, os.ErrNotExist) {
			err = os.Remove(destinations[i])
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// switchRecords makes manifest hold the records of the install into target.
// The records of an install into another target are parked under their own
// target rather than taken as this install's previous state, so nothing that
// install wrote is pruned or stripped, and installing into it again resumes
// them.
func switchRecords(manifest, target string) error {
	data, err := os.ReadFile(manifest)
	if errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(parked(manifest, target)); err != nil {
			return nil
		}
		return moveRecords(parked(manifest, target), manifest)
	}
	if err != nil {
		return err
	}
	var record installRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("%s: %w", manifest, err)
	}
	if record.Target == "" || record.Target == target {
		return nil
	}
	if err := moveRecords(manifest, parked(manifest, record.Target)); err != nil {
		return err
	}
	if _, err := os.Stat(parked(manifest, target)); err != nil {
		return nil
	}
	return moveRecords(parked(manifest, target), manifest)
}

// installTree installs one harness's files: whole files into target or the
// project or home root their profile names, and merged config files unit by
// unit.
func installTree(cmd *cobra.Command, harness, target, project string, files map[string]string, manifest string, force, strict bool, revisions map[string]string, where map[string]profile.Placement) error {
	if err := switchRecords(manifest, target); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	roots := map[string]string{"project": project, "home": home}
	whole := map[string]string{}
	rooted := map[string]map[string]string{"project": {}, "home": {}}
	var merged []units.File
	schemas := profile.NewSchemas()
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		p, ok := where[rel]
		switch {
		case !ok || p.Root == "harness" && !p.Merge:
			whole[rel] = files[rel]
		case roots[p.Root] == "" && p.Root != "harness":
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s: %s belongs to the %s root, which this install has none of; skipped\n", harness, rel, p.Root)
		case p.Merge:
			dir := target
			if p.Root != "harness" {
				dir = roots[p.Root]
			}
			data, err := os.ReadFile(files[rel])
			if err != nil {
				return err
			}
			value, err := units.Decode(p.Format, data)
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			file := units.File{Path: filepath.Join(dir, rel), Format: p.Format, Entries: p.Entries, Value: value, Strict: strict}
			if schema := p.Schema; schema != "" {
				file.Validate = func(doc map[string]any) error { return schemas.Validate(schema, doc) }
			}
			merged = append(merged, file)
		default:
			rooted[p.Root][rel] = files[rel]
		}
	}
	if !force {
		if err := installConflicts(harness, target, whole, manifest, roots, rooted, merged, sibling(manifest, "units")); err != nil {
			return err
		}
	}
	if err := installFiles(cmd, harness, target, whole, manifest, force, revisions); err != nil {
		return err
	}
	for _, root := range []string{"project", "home"} {
		rootManifest := sibling(manifest, root)
		if len(rooted[root]) == 0 && !exists(rootManifest) {
			continue
		}
		if err := installFiles(cmd, harness, roots[root], rooted[root], rootManifest, force, nil); err != nil {
			return err
		}
	}
	return installUnits(cmd, harness, merged, sibling(manifest, "units"), force)
}

// installConflicts checks every destination of one harness before any is
// written, so a conflict anywhere leaves the install untouched.
func installConflicts(harness, target string, whole map[string]string, manifest string, roots map[string]string, rooted map[string]map[string]string, merged []units.File, unitsManifest string) error {
	var problems []string
	check := func(dir string, files map[string]string, manifest string) error {
		_, conflicts, err := fileConflicts(dir, files, manifest)
		for _, rel := range conflicts {
			problems = append(problems, filepath.Join(dir, rel))
		}
		return err
	}
	if err := check(target, whole, manifest); err != nil {
		return err
	}
	for _, root := range []string{"project", "home"} {
		if err := check(roots[root], rooted[root], sibling(manifest, root)); err != nil {
			return err
		}
	}
	previous, err := readUnits(unitsManifest)
	if err != nil {
		return err
	}
	if err := units.Check(merged, previous); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s: not installed by henia or edited since: %s; edit the Henia config to match, move the files away, or pass --force", harness, strings.Join(problems, "; "))
	}
	return nil
}

func readUnits(manifest string) (units.Record, error) {
	var record units.Record
	data, err := os.ReadFile(manifest)
	if errors.Is(err, os.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("%s: %w", manifest, err)
	}
	return record, nil
}

func uninstallUnits(manifest string) error {
	previous, err := readUnits(manifest)
	if err != nil || previous == nil {
		return err
	}
	if _, _, err := units.Apply(nil, previous, false); err != nil {
		return err
	}
	return os.Remove(manifest)
}

func installUnits(cmd *cobra.Command, harness string, files []units.File, manifest string, force bool) error {
	previous, err := readUnits(manifest)
	if err != nil {
		return err
	}
	if len(files) == 0 && len(previous) == 0 {
		return nil
	}
	record, warnings, err := units.Apply(files, previous, force)
	if err != nil {
		return fmt.Errorf("%w; edit the Henia config to match, or pass --force to replace them", err)
	}
	for _, warning := range warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s: %s\n", harness, warning)
	}
	if len(record) == 0 {
		if err := os.Remove(manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifest, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Merged config for %s into %s\n", harness, strings.Join(slices.Sorted(maps.Keys(record)), ", "))
	return nil
}
