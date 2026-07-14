// The analysis pipeline: discover files, route each to its scanner,
// filter, and aggregate into a census.Report. Pure except for reading the
// tree — no subprocesses, no network.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JaydenCJ/xenolist/internal/census"
	"github.com/JaydenCJ/xenolist/internal/finding"
	"github.com/JaydenCJ/xenolist/internal/scan"
	"github.com/JaydenCJ/xenolist/internal/walk"
)

type analyzeOptions struct {
	Path        string
	Include     []string
	Exclude     []string
	Kinds       map[finding.Kind]bool // empty = all kinds
	MaxFileSize int64
}

func analyze(opts analyzeOptions) (census.Report, error) {
	info, err := os.Stat(opts.Path)
	if err != nil {
		return census.Report{}, err
	}
	if !info.IsDir() {
		return census.Report{}, fmt.Errorf("%s is not a directory", opts.Path)
	}
	files, err := walk.Collect(walk.Options{
		Root: opts.Path, Include: opts.Include, Exclude: opts.Exclude,
		MaxFileSize: opts.MaxFileSize,
	})
	if err != nil {
		return census.Report{}, err
	}
	fileCounts := map[string]int{}
	var findings []finding.Finding
	for _, f := range files {
		data, err := os.ReadFile(f.Abs)
		if err != nil {
			return census.Report{}, err
		}
		fileCounts[f.Label]++
		src := string(data)
		switch f.Kind {
		case walk.KindYAML:
			findings = append(findings, scan.ScanYAML(f.Rel, src)...)
		case walk.KindDocker:
			findings = append(findings, scan.ScanDockerfile(f.Rel, src)...)
		case walk.KindShell:
			findings = append(findings, scan.ScanShellText(f.Rel, src, 1)...)
		case walk.KindMake:
			findings = append(findings, scan.ScanMakefile(f.Rel, src)...)
		case walk.KindPackageJSON:
			findings = append(findings, scan.ScanPackageJSON(f.Rel, src)...)
		}
	}
	if len(opts.Kinds) > 0 {
		kept := findings[:0]
		for _, f := range findings {
			if opts.Kinds[f.Kind] {
				kept = append(kept, f)
			}
		}
		findings = kept
	}
	abs, err := filepath.Abs(opts.Path)
	if err != nil {
		return census.Report{}, err
	}
	return census.Build(filepath.Base(abs), fileCounts, findings), nil
}
