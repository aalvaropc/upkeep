package fixer

import (
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"strings"
)

func ApplyEdits(dir string, edits []Edit) error {
	if len(edits) == 0 {
		return errors.New("there are no edits to apply")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	files := map[string][]byte{}
	originals := map[string][]byte{}
	modes := map[string]fs.FileMode{}
	var order []string
	for i, e := range edits {
		clean, err := editablePath(e.Path)
		if err != nil {
			return fmt.Errorf("edit %d: %w", i+1, err)
		}
		if why := protected(clean); why != "" {
			return fmt.Errorf("edit %d: %s", i+1, why)
		}
		if e.Old == "" {
			return fmt.Errorf("edit %d: old must not be empty", i+1)
		}
		data, seen := files[clean]
		if !seen {
			if data, err = root.ReadFile(clean); err != nil {
				return fmt.Errorf("edit %d: %s cannot be read", i+1, clean)
			}
			info, err := root.Stat(clean)
			if err != nil {
				return fmt.Errorf("edit %d: %s cannot be read", i+1, clean)
			}
			originals[clean] = data
			modes[clean] = info.Mode().Perm()
			order = append(order, clean)
		}
		text := string(data)
		if n := strings.Count(text, e.Old); n != 1 {
			return fmt.Errorf("edit %d: old was found %d times in %s, it must match exactly once", i+1, n, clean)
		}
		updated := []byte(strings.Replace(text, e.Old, e.New, 1))
		if countComments(updated) > countComments(data) {
			return fmt.Errorf("edit %d adds a comment to %s; the code must have no comments", i+1, clean)
		}
		if formatted, ferr := format.Source(updated); ferr == nil {
			updated = formatted
		}
		files[clean] = updated
	}

	for _, clean := range order {
		if _, err := format.Source(files[clean]); err != nil {
			return fmt.Errorf("%s no longer parses after the edits: %s", clean, firstLine(err.Error()))
		}
	}
	for i, clean := range order {
		if err := root.WriteFile(clean, files[clean], modes[clean]); err != nil {
			for _, done := range order[:i] {
				_ = root.WriteFile(done, originals[done], modes[done])
			}
			return fmt.Errorf("%s cannot be written", clean)
		}
	}
	return nil
}
