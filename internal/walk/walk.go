// Package walk discovers and classifies the files worth scanning: GitHub
// workflows and composite actions, Dockerfiles, compose files, GitLab and
// CircleCI configs, shell scripts, Makefiles, and package.json manifests.
// Dependency and build-output directories are skipped — code in
// node_modules is a package-manager concern, not a census entry.
package walk

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileKind selects the scanner a file is routed to.
type FileKind int

const (
	KindYAML FileKind = iota // workflows, actions, compose, GitLab/CircleCI
	KindDocker
	KindShell
	KindMake
	KindPackageJSON
)

// File is one classified candidate.
type File struct {
	Rel   string // slash-separated path relative to the root
	Abs   string
	Kind  FileKind
	Label string // human category for the report, e.g. "workflow"
}

// Options controls discovery.
type Options struct {
	Root        string
	Include     []string // if non-empty, keep only matching files
	Exclude     []string
	MaxFileSize int64 // skip larger files; 0 means the 1 MiB default
}

// skipDirs are never descended into.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".venv": true, "venv": true,
	"__pycache__": true, ".tox": true, ".cache": true, ".next": true,
	"third_party": true,
}

// Collect walks root and returns every classified file, sorted by path.
func Collect(opts Options) ([]File, error) {
	maxSize := opts.MaxFileSize
	if maxSize == 0 {
		maxSize = 1 << 20
	}
	var out []File
	err := filepath.WalkDir(opts.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != opts.Root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil // never follow symlinks out of the tree
		}
		rel, err := filepath.Rel(opts.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		kind, label, ok := Classify(rel)
		if !ok {
			return nil
		}
		for _, pat := range opts.Exclude {
			if Match(pat, rel) {
				return nil
			}
		}
		if len(opts.Include) > 0 {
			keep := false
			for _, pat := range opts.Include {
				if Match(pat, rel) {
					keep = true
					break
				}
			}
			if !keep {
				return nil
			}
		}
		if info, err := d.Info(); err == nil && info.Size() > maxSize {
			return nil
		}
		out = append(out, File{Rel: rel, Abs: path, Kind: kind, Label: label})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// Classify maps a repo-relative path to a scanner and a report label.
func Classify(rel string) (FileKind, string, bool) {
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	lower := strings.ToLower(base)
	switch {
	case lower == "dockerfile" || lower == "containerfile" ||
		strings.HasPrefix(lower, "dockerfile.") || strings.HasSuffix(lower, ".dockerfile"):
		return KindDocker, "dockerfile", true
	case strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml"):
		switch {
		case strings.Contains(rel, ".github/workflows/"):
			return KindYAML, "workflow", true
		case lower == "action.yml" || lower == "action.yaml":
			return KindYAML, "action", true
		case strings.HasPrefix(lower, "docker-compose") || lower == "compose.yml" || lower == "compose.yaml":
			return KindYAML, "compose file", true
		case lower == ".gitlab-ci.yml" || lower == ".gitlab-ci.yaml":
			return KindYAML, "ci config", true
		case strings.HasSuffix(rel, ".circleci/config.yml") || strings.HasSuffix(rel, ".circleci/config.yaml"):
			return KindYAML, "ci config", true
		}
		return 0, "", false
	case strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".bash"):
		return KindShell, "shell script", true
	case base == "Makefile" || base == "makefile" || base == "GNUmakefile" ||
		strings.HasSuffix(lower, ".mk"):
		return KindMake, "makefile", true
	case lower == "package.json":
		return KindPackageJSON, "package.json", true
	}
	return 0, "", false
}
