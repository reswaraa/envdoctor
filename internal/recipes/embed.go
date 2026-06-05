// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package recipes

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed library
var embedded embed.FS

// DefaultLibrary returns the Library bundled with the binary. Recipes
// ship as YAML files under internal/recipes/library/ and are embedded
// at compile time via go:embed; no runtime fetch.
func DefaultLibrary() (*Library, error) {
	return LoadFS(embedded, "library")
}

// Library is the parsed recipe set with O(1) lookup by recipe ID and
// O(1) probe → []Recipe index.
type Library struct {
	Recipes []Recipe
	byID    map[string]int
	byProbe map[string][]int
}

// Lookup returns the Recipe with the given ID, or false.
func (l *Library) Lookup(id string) (Recipe, bool) {
	i, ok := l.byID[id]
	if !ok {
		return Recipe{}, false
	}
	return l.Recipes[i], true
}

// ForProbe returns all recipes targeting the given probe ID.
func (l *Library) ForProbe(probeID string) []Recipe {
	idxs := l.byProbe[probeID]
	out := make([]Recipe, len(idxs))
	for i, idx := range idxs {
		out[i] = l.Recipes[idx]
	}
	return out
}

// LoadFS reads every *.yaml under root in fsys and returns a validated
// Library. Validation errors include the file name. Failure modes:
//
//   - YAML parse error
//   - missing required fields (id, probe, fixes[].id, fixes[].command)
//   - duplicate recipe ID across files
//   - duplicate fix ID within a recipe
func LoadFS(fsys fs.FS, root string) (*Library, error) {
	var (
		recipes []Recipe
		files   []string
	)
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := path.Ext(p)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	for _, p := range files {
		b, readErr := fs.ReadFile(fsys, p)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", p, readErr)
		}
		var r Recipe
		if uErr := yaml.Unmarshal(b, &r); uErr != nil {
			return nil, fmt.Errorf("parse %s: %w", p, uErr)
		}
		if vErr := validateRecipe(p, r); vErr != nil {
			return nil, vErr
		}
		recipes = append(recipes, r)
	}

	sort.Slice(recipes, func(i, j int) bool { return recipes[i].ID < recipes[j].ID })

	lib := &Library{
		Recipes: recipes,
		byID:    make(map[string]int, len(recipes)),
		byProbe: map[string][]int{},
	}
	for i, r := range recipes {
		if _, dup := lib.byID[r.ID]; dup {
			return nil, fmt.Errorf("duplicate recipe id %q", r.ID)
		}
		lib.byID[r.ID] = i
		lib.byProbe[r.Probe] = append(lib.byProbe[r.Probe], i)
	}
	return lib, nil
}

func validateRecipe(file string, r Recipe) error {
	if r.ID == "" {
		return fmt.Errorf("%s: missing 'id'", file)
	}
	if r.Probe == "" {
		return fmt.Errorf("%s: recipe %q missing 'probe'", file, r.ID)
	}
	if len(r.Fixes) == 0 {
		return fmt.Errorf("%s: recipe %q has no 'fixes'", file, r.ID)
	}
	seen := map[string]bool{}
	for i, f := range r.Fixes {
		if f.ID == "" {
			return fmt.Errorf("%s: recipe %q fix #%d missing 'id'", file, r.ID, i+1)
		}
		if f.Command == "" {
			return fmt.Errorf("%s: recipe %q fix %q missing 'command'", file, r.ID, f.ID)
		}
		if seen[f.ID] {
			return fmt.Errorf("%s: recipe %q has duplicate fix id %q", file, r.ID, f.ID)
		}
		seen[f.ID] = true
		switch f.Class {
		case ClassSafe, ClassShared, ClassDestructive, ClassPrivileged:
		case "":
			return fmt.Errorf("%s: recipe %q fix %q missing 'class'", file, r.ID, f.ID)
		default:
			return fmt.Errorf("%s: recipe %q fix %q has unknown class %q", file, r.ID, f.ID, f.Class)
		}
	}
	return nil
}
