package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia/internal/markup"
)

func newFmtCommand() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "fmt [paths...]",
		Short: "Rewrite directive fence lengths to their canonical nesting",
		Long: `Rewrite directive fence lengths from the document tree: each outer fence is
one colon longer than the longest fence it contains, the innermost ':::'.
Directories are searched for Markdown files; without paths, the current
directory. --check lists the files that would change and writes nothing.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{"."}
			}
			files, err := markdownFiles(args)
			var changed []string
			for _, file := range files {
				differs, ferr := formatFile(file, check)
				if ferr != nil {
					err = errors.Join(err, ferr)
					continue
				}
				if differs {
					changed = append(changed, file)
				}
			}
			if check {
				for _, file := range changed {
					if _, werr := fmt.Fprintln(cmd.OutOrStdout(), file); werr != nil {
						return errors.Join(err, werr)
					}
				}
				if len(changed) > 0 {
					err = errors.Join(err, fmt.Errorf("%d file(s) would change", len(changed)))
				}
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "List files whose fences would change and exit unsuccessfully; write nothing")
	return cmd
}

func markdownFiles(paths []string) ([]string, error) {
	var files []string
	var errs error
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if !d.IsDir() && filepath.Ext(p) == ".md" {
				files = append(files, p)
			}
			return nil
		})
		errs = errors.Join(errs, err)
	}
	return files, errs
}

func formatFile(file string, check bool) (bool, error) {
	info, err := os.Stat(file)
	if err != nil {
		return false, err
	}
	source, err := os.ReadFile(file)
	if err != nil {
		return false, err
	}
	doc, err := markup.Parse(file, source)
	if err != nil {
		return false, located(file, err)
	}
	formatted := markup.Write(doc)
	if bytes.Equal(formatted, source) {
		return false, nil
	}
	if check {
		return true, nil
	}
	if err := os.WriteFile(file, formatted, info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}

func located(file string, err error) error {
	list, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return fmt.Errorf("%s:%w", file, err)
	}
	var errs []error
	for _, e := range list.Unwrap() {
		errs = append(errs, fmt.Errorf("%s:%w", file, e))
	}
	return errors.Join(errs...)
}
