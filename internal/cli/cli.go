// Package cli implements the xenolist command-line interface. Run takes
// argv and two writers and returns an exit code, so the whole surface is
// testable in-process without building a binary.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/JaydenCJ/xenolist/internal/finding"
	"github.com/JaydenCJ/xenolist/internal/render"
	"github.com/JaydenCJ/xenolist/internal/version"
)

// Exit codes. Documented in the README; `check` uses ExitBreach as its
// machine-readable verdict.
const (
	ExitOK      = 0
	ExitBreach  = 1
	ExitUsage   = 2
	ExitRuntime = 3
)

// Run dispatches argv and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runScan(nil, stdout, stderr)
	}
	switch args[0] {
	case "scan":
		return runScan(args[1:], stdout, stderr)
	case "list":
		return runList(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "xenolist %s\n", version.Version)
		return ExitOK
	case "help", "--help", "-h":
		usage(stdout)
		return ExitOK
	default:
		if strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(stderr, "xenolist: unknown flag %q before a subcommand\n\n", args[0])
			usage(stderr)
			return ExitUsage
		}
		// Bare path: treat as `scan <path>`.
		return runScan(args, stdout, stderr)
	}
}

// multiFlag is a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// sharedFlags are common to scan, list, and check.
type sharedFlags struct {
	format      string
	include     multiFlag
	exclude     multiFlag
	kinds       multiFlag
	maxFileSize int64
}

func (s *sharedFlags) register(fs *flag.FlagSet, withFormat bool) {
	if withFormat {
		fs.StringVar(&s.format, "format", "text", "output format: text, json, or markdown")
	}
	fs.Var(&s.include, "include", "only scan files matching this glob (repeatable)")
	fs.Var(&s.exclude, "exclude", "skip files matching this glob, e.g. 'examples/**' (repeatable)")
	fs.Var(&s.kinds, "kind", "only report this kind (repeatable): github-action, container-image, package-exec, pipe-to-shell, remote-download")
	fs.Int64Var(&s.maxFileSize, "max-file-size", 1<<20, "skip files larger than this many bytes")
}

func (s *sharedFlags) toOptions(path string) (analyzeOptions, error) {
	opts := analyzeOptions{
		Path: path, Include: s.include, Exclude: s.exclude,
		MaxFileSize: s.maxFileSize,
	}
	if len(s.kinds) > 0 {
		opts.Kinds = map[finding.Kind]bool{}
		for _, k := range s.kinds {
			known := false
			for _, kk := range finding.AllKinds {
				if string(kk) == k {
					known = true
					break
				}
			}
			if !known {
				return opts, fmt.Errorf("unknown --kind %q", k)
			}
			opts.Kinds[finding.Kind(k)] = true
		}
	}
	return opts, nil
}

func runScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var sf sharedFlags
	sf.register(fs, true)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	path, code := onePath(fs.Args(), stderr)
	if code != ExitOK {
		return code
	}
	if sf.format != "text" && sf.format != "json" && sf.format != "markdown" {
		fmt.Fprintf(stderr, "xenolist: unknown --format %q (want text, json, or markdown)\n", sf.format)
		return ExitUsage
	}
	opts, err := sf.toOptions(path)
	if err != nil {
		fmt.Fprintf(stderr, "xenolist: %v\n", err)
		return ExitUsage
	}
	report, err := analyze(opts)
	if err != nil {
		fmt.Fprintf(stderr, "xenolist: %v\n", err)
		return ExitRuntime
	}
	switch sf.format {
	case "json":
		if err := render.JSON(stdout, report); err != nil {
			fmt.Fprintf(stderr, "xenolist: %v\n", err)
			return ExitRuntime
		}
	case "markdown":
		render.Markdown(stdout, report)
	default:
		render.Scan(stdout, report)
	}
	return ExitOK
}

func runList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var sf sharedFlags
	sf.register(fs, true)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	path, code := onePath(fs.Args(), stderr)
	if code != ExitOK {
		return code
	}
	if sf.format != "text" && sf.format != "json" {
		fmt.Fprintf(stderr, "xenolist: unknown --format %q (want text or json)\n", sf.format)
		return ExitUsage
	}
	opts, err := sf.toOptions(path)
	if err != nil {
		fmt.Fprintf(stderr, "xenolist: %v\n", err)
		return ExitUsage
	}
	report, err := analyze(opts)
	if err != nil {
		fmt.Fprintf(stderr, "xenolist: %v\n", err)
		return ExitRuntime
	}
	if sf.format == "json" {
		if err := render.JSON(stdout, report); err != nil {
			fmt.Fprintf(stderr, "xenolist: %v\n", err)
			return ExitRuntime
		}
		return ExitOK
	}
	render.List(stdout, report)
	return ExitOK
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var sf sharedFlags
	sf.register(fs, false)
	maxSources := fs.Int("max-sources", -1, "fail when unique external sources exceed this count")
	maxFloating := fs.Int("max-floating", -1, "fail when floating (unpinned) sources exceed this count")
	var allowHosts multiFlag
	fs.Var(&allowHosts, "allow-host", "allowed host; any source from another host fails (repeatable)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	path, code := onePath(fs.Args(), stderr)
	if code != ExitOK {
		return code
	}
	if *maxSources < 0 && *maxFloating < 0 && len(allowHosts) == 0 {
		fmt.Fprintf(stderr, "xenolist check: set --max-sources, --max-floating, and/or --allow-host\n")
		return ExitUsage
	}
	opts, err := sf.toOptions(path)
	if err != nil {
		fmt.Fprintf(stderr, "xenolist: %v\n", err)
		return ExitUsage
	}
	report, err := analyze(opts)
	if err != nil {
		fmt.Fprintf(stderr, "xenolist: %v\n", err)
		return ExitRuntime
	}

	breached := false
	verdict := func(name string, actual, limit int) {
		status := "ok"
		if actual > limit {
			status = "BREACH"
			breached = true
		}
		fmt.Fprintf(stdout, "%-18s %4d  (limit %d)  %s\n", name, actual, limit, status)
	}
	if *maxSources >= 0 {
		verdict("sources", len(report.Sources), *maxSources)
	}
	if *maxFloating >= 0 {
		_, _, floating := report.PinCounts()
		verdict("floating sources", floating, *maxFloating)
	}
	if len(allowHosts) > 0 {
		allowed := map[string]bool{}
		for _, h := range allowHosts {
			allowed[h] = true
		}
		for _, s := range report.Sources {
			if allowed[s.Host] {
				continue
			}
			occ := s.Occurrences[0]
			fmt.Fprintf(stdout, "host %s not allowed: %s (%s:%d)  BREACH\n",
				s.Host, s.Ref, occ.File, occ.Line)
			breached = true
		}
	}
	if breached {
		fmt.Fprintf(stdout, "check: FAIL\n")
		return ExitBreach
	}
	fmt.Fprintf(stdout, "check: PASS\n")
	return ExitOK
}

// onePath extracts the optional single positional path argument.
func onePath(rest []string, stderr io.Writer) (string, int) {
	switch len(rest) {
	case 0:
		return ".", ExitOK
	case 1:
		return rest[0], ExitOK
	default:
		fmt.Fprintf(stderr, "xenolist: expected at most one path argument, got %d\n", len(rest))
		return "", ExitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `xenolist %s — every piece of external code your repo executes, one report

Usage:
  xenolist [scan] [flags] [path]   census summary: sources, kinds, hosts (default)
  xenolist list  [flags] [path]    every occurrence with its evidence line
  xenolist check [flags] [path]    enforce limits (exit 1 on breach)
  xenolist version                 print the version

Scan/list flags:
  --format FORMAT       text (default), json, or markdown (list: text/json)
  --include GLOB        only scan matching files (repeatable)
  --exclude GLOB        skip matching files, e.g. 'examples/**' (repeatable)
  --kind KIND           only report this kind (repeatable)
  --max-file-size N     skip files larger than N bytes (default 1048576)

Check flags (plus all scan flags except --format):
  --max-sources N       fail when unique external sources exceed N
  --max-floating N      fail when floating (unpinned) sources exceed N
  --allow-host HOST     allowlist a host; anything else fails (repeatable)

Kinds: github-action, container-image, package-exec, pipe-to-shell, remote-download
Exit codes: 0 ok · 1 check breach · 2 usage error · 3 runtime error
`, version.Version)
}
