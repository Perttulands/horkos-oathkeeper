package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/config"
	"github.com/perttulands/oathkeeper/pkg/doctor"
	"github.com/perttulands/oathkeeper/pkg/scanner"
)

const version = "2.0.0"

const usage = `Oathkeeper — Beads-native commitment guard

Usage:
  oathkeeper <command> [flags]

Commands:
  serve    Start the HTTP server and daemon
  scan     Batch scan a transcript file for commitments
  list     List open oathkeeper beads
  stats    Show commitment statistics
  resolve  Resolve a commitment bead
  doctor   Run health checks on all dependencies

Flags:
  --config PATH  Config file (default: ~/.config/oathkeeper/oathkeeper.toml)
  --help         Show this help
  --version      Show version
`

var tagValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

const (
	scanUsage    = "Usage: oathkeeper scan <file> [--format text|json] [--json]"
	listUsage    = "Usage: oathkeeper list [--status open|closed|all] [--category CATEGORY] [--since DURATION] [--tag a,b,c] [--json]"
	statsUsage   = "Usage: oathkeeper stats [--json]"
	resolveUsage = "Usage: oathkeeper resolve <bead-id> [reason] [--reason REASON] [--json]"
	doctorUsage  = "Usage: oathkeeper doctor [--json]"
	serveUsage   = "Usage: oathkeeper serve [--tag a,b,c]"
)

type serveOptions struct {
	extraTags []string
}

type scanOptions struct {
	file   string
	format string
	json   bool
}

type listOptions struct {
	status   string
	category string
	since    time.Duration
	tags     []string
	json     bool
}

type statsOptions struct {
	json bool
}

type resolveOptions struct {
	beadID string
	reason string
	json   bool
}

type doctorOptions struct {
	json bool
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	cmd := os.Args[1]

	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		fmt.Print(usage)
		return
	}

	if cmd == "--version" || cmd == "version" {
		fmt.Printf("oathkeeper v%s\n", version)
		return
	}

	// Global flags: extract --config from args after the subcommand
	configPath, subArgs, err := extractConfigFlag(os.Args[2:])
	if err != nil {
		exitWithError(err.Error(), nil, wantsJSON(subArgs))
	}

	switch cmd {
	case "serve":
		runServe(configPath, subArgs)
	case "scan":
		runScan(subArgs)
	case "list":
		runList(configPath, subArgs)
	case "stats":
		runStats(configPath, subArgs)
	case "resolve":
		runResolve(configPath, subArgs)
	case "doctor":
		runDoctor(configPath, subArgs)
	default:
		exitWithError(fmt.Sprintf("Unknown command %q.", cmd), nil, wantsJSON(os.Args[2:]))
	}
}

// extractConfigFlag pulls --config VALUE from args, returning the config path
// and remaining args with --config and its value removed.
func extractConfigFlag(args []string) (string, []string, error) {
	configPath := ""
	var remaining []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("missing value for --config")
			}
			if configPath != "" {
				return "", nil, fmt.Errorf("--config provided more than once")
			}
			value := strings.TrimSpace(args[i+1])
			if value == "" {
				return "", nil, fmt.Errorf("--config cannot be empty")
			}
			configPath = value
			i++ // skip value
			continue
		case strings.HasPrefix(arg, "--config="):
			if configPath != "" {
				return "", nil, fmt.Errorf("--config provided more than once")
			}
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			if value == "" {
				return "", nil, fmt.Errorf("--config cannot be empty")
			}
			configPath = value
			continue
		}
		remaining = append(remaining, arg)
	}

	return configPath, remaining, nil
}

func loadConfig(configPath string) *config.Config {
	if configPath == "" {
		configPath = config.ExpandPath(config.DefaultConfigPath())
	} else {
		configPath = config.ExpandPath(configPath)
	}
	return config.LoadOrDefault(configPath)
}

func runServe(configPath string, args []string) {
	opts, err := parseServeArgs(args)
	if err != nil {
		exitWithError(err.Error(), nil, wantsJSON(args))
	}
	startServer(configPath, opts.extraTags)
}

func runScan(args []string) {
	opts, err := parseScanArgs(args)
	if err != nil {
		exitWithError(err.Error(), nil, wantsJSON(args))
	}

	if _, err := os.Stat(opts.file); os.IsNotExist(err) {
		exitWithError(fmt.Sprintf("Transcript file %q was not found.", opts.file), err, opts.json)
	}
	if _, err := os.Stat(opts.file); err != nil {
		exitWithError(fmt.Sprintf("Transcript file %q is not readable.", opts.file), err, opts.json)
	}

	results, err := scanner.ScanFile(opts.file)
	if err != nil {
		exitWithError("Could not scan the transcript file.", err, opts.json)
	}

	switch opts.format {
	case "json":
		fmt.Print(scanner.FormatScanResultsJSON(results))
	default:
		fmt.Print(scanner.FormatScanResults(results))
	}
}

func runList(configPath string, args []string) {
	opts, err := parseListArgs(args)
	if err != nil {
		exitWithError(err.Error(), nil, wantsJSON(args))
	}

	cfg := loadConfig(configPath)
	store := beads.NewBeadStore(cfg.Verification.BeadsCommand)

	filter := beads.Filter{Status: opts.status}
	if opts.status == "all" {
		filter.Status = ""
	}
	if opts.category != "" {
		filter.Category = opts.category
	}
	if opts.since > 0 {
		filter.Since = time.Now().Add(-opts.since)
	}

	list, err := store.List(filter)
	if err != nil {
		exitWithError("Could not list commitment beads.", err, opts.json)
	}

	if len(opts.tags) > 0 {
		list = filterByTags(list, opts.tags)
	}

	if opts.json {
		writeJSON(os.Stdout, map[string]interface{}{
			"commitments": list,
			"count":       len(list),
		})
		return
	}

	if len(list) == 0 {
		fmt.Println("No matching oathkeeper commitments.")
		return
	}

	// Print header
	fmt.Printf("%-10s  %-12s  %-40s  %s\n", "ID", "STATUS", "TITLE", "TAGS")
	fmt.Printf("%-10s  %-12s  %-40s  %s\n", "---", "------", "-----", "----")
	for _, b := range list {
		id := b.ID
		if len(id) > 10 {
			id = id[:10]
		}
		title := b.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		tags := strings.Join(b.Tags, ", ")
		fmt.Printf("%-10s  %-12s  %-40s  %s\n", id, b.Status, title, tags)
	}
	fmt.Printf("\n%d commitment(s)\n", len(list))
}

func runStats(configPath string, args []string) {
	_, err := parseStatsArgs(args)
	if err != nil {
		exitWithError(err.Error(), nil, wantsJSON(args))
	}

	cfg := loadConfig(configPath)
	store := beads.NewBeadStore(cfg.Verification.BeadsCommand)

	list, err := store.List(beads.Filter{})
	if err != nil {
		exitWithError("Could not load commitment statistics.", err, wantsJSON(args))
	}

	total := len(list)
	open := 0
	resolved := 0
	byCategory := map[string]int{}

	for _, b := range list {
		switch strings.ToLower(strings.TrimSpace(b.Status)) {
		case "open":
			open++
		case "closed":
			resolved++
		}
		for _, tag := range b.Tags {
			normalized := strings.ToLower(strings.TrimSpace(tag))
			if normalized == "" || normalized == "oathkeeper" || strings.HasPrefix(normalized, "session-") {
				continue
			}
			byCategory[normalized]++
			break
		}
	}

	out := map[string]interface{}{
		"total":       total,
		"open":        open,
		"resolved":    resolved,
		"by_category": byCategory,
	}
	writeJSON(os.Stdout, out)
}

func runResolve(configPath string, args []string) {
	opts, err := parseResolveArgs(args)
	if err != nil {
		exitWithError(err.Error(), nil, wantsJSON(args))
	}

	cfg := loadConfig(configPath)
	store := beads.NewBeadStore(cfg.Verification.BeadsCommand)

	if err := store.Resolve(opts.beadID, opts.reason); err != nil {
		exitWithError(fmt.Sprintf("Could not resolve bead %q.", opts.beadID), err, opts.json)
	}

	if opts.json {
		writeJSON(os.Stdout, map[string]interface{}{
			"bead_id":  opts.beadID,
			"resolved": true,
			"reason":   opts.reason,
		})
		return
	}
	fmt.Printf("Resolved %s: %s\n", opts.beadID, opts.reason)
}

func runDoctor(configPath string, args []string) {
	opts, err := parseDoctorArgs(args)
	if err != nil {
		exitWithError(err.Error(), nil, wantsJSON(args))
	}

	cfg := loadConfig(configPath)

	resolvedConfigPath := configPath
	if resolvedConfigPath == "" {
		resolvedConfigPath = config.DefaultConfigPath()
	}
	resolvedConfigPath = config.ExpandPath(resolvedConfigPath)

	results := doctor.RunChecks(doctor.Config{
		Version:       version,
		DBPath:        config.ExpandPath(cfg.Storage.DBPath),
		ConfigPath:    resolvedConfigPath,
		OpenClawURL:   cfg.OpenClaw.APIURL,
		BeadsCommand:  cfg.Verification.BeadsCommand,
		TmuxCommand:   cfg.Verification.TmuxCommand,
		ClaudeCommand: cfg.LLM.Command,
		ArgusWebhook:  cfg.Alerts.TelegramWebhook,
	})

	if opts.json {
		writeJSON(os.Stdout, map[string]interface{}{"checks": results})
		return
	}

	fmt.Println(doctor.FormatReport(results))
}

func parseServeArgs(args []string) (serveOptions, error) {
	fs := newFlagSet("serve")
	tagValue := fs.String("tag", "", "Comma-separated tags to include when creating beads")
	if err := parseFlags(fs, args, serveUsage); err != nil {
		return serveOptions{}, err
	}
	if fs.NArg() > 0 {
		return serveOptions{}, fmt.Errorf("unexpected argument(s) for serve: %s", strings.Join(fs.Args(), " "))
	}

	tags, err := parseTagValues(*tagValue)
	if err != nil {
		return serveOptions{}, fmt.Errorf("invalid --tag value: %w", err)
	}
	return serveOptions{extraTags: tags}, nil
}

func parseScanArgs(args []string) (scanOptions, error) {
	fs := newFlagSet("scan")
	format := fs.String("format", "text", "Output format: text or json")
	jsonOut := fs.Bool("json", false, "Output machine-readable JSON")
	if err := parseFlags(fs, args, scanUsage); err != nil {
		return scanOptions{}, err
	}
	if fs.NArg() != 1 {
		return scanOptions{}, fmt.Errorf(scanUsage)
	}

	chosenFormat := strings.ToLower(strings.TrimSpace(*format))
	if *jsonOut {
		chosenFormat = "json"
	}
	if chosenFormat != "text" && chosenFormat != "json" {
		return scanOptions{}, fmt.Errorf("invalid --format %q (allowed: text, json)", *format)
	}

	file := strings.TrimSpace(fs.Arg(0))
	if file == "" {
		return scanOptions{}, fmt.Errorf("scan file path cannot be empty")
	}

	return scanOptions{
		file:   file,
		format: chosenFormat,
		json:   *jsonOut || chosenFormat == "json",
	}, nil
}

func parseListArgs(args []string) (listOptions, error) {
	fs := newFlagSet("list")
	status := fs.String("status", "open", "Status filter: open, closed, all")
	category := fs.String("category", "", "Category filter (single tag)")
	since := fs.String("since", "", "Only include commitments newer than this duration (e.g. 24h)")
	tags := fs.String("tag", "", "Comma-separated tag filter")
	jsonOut := fs.Bool("json", false, "Output machine-readable JSON")
	if err := parseFlags(fs, args, listUsage); err != nil {
		return listOptions{}, err
	}
	if fs.NArg() > 0 {
		return listOptions{}, fmt.Errorf("unexpected argument(s) for list: %s", strings.Join(fs.Args(), " "))
	}

	normalizedStatus := strings.ToLower(strings.TrimSpace(*status))
	switch normalizedStatus {
	case "open", "closed", "all":
	default:
		return listOptions{}, fmt.Errorf("invalid --status %q (allowed: open, closed, all)", *status)
	}

	normalizedCategory := strings.TrimSpace(*category)
	if normalizedCategory != "" && !tagValuePattern.MatchString(normalizedCategory) {
		return listOptions{}, fmt.Errorf("invalid --category %q", *category)
	}

	var sinceDuration time.Duration
	if strings.TrimSpace(*since) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(*since))
		if err != nil {
			return listOptions{}, fmt.Errorf("invalid --since value %q (example: 24h)", *since)
		}
		if parsed <= 0 {
			return listOptions{}, fmt.Errorf("--since must be greater than 0")
		}
		sinceDuration = parsed
	}

	parsedTags, err := parseTagValues(*tags)
	if err != nil {
		return listOptions{}, fmt.Errorf("invalid --tag value: %w", err)
	}

	return listOptions{
		status:   normalizedStatus,
		category: normalizedCategory,
		since:    sinceDuration,
		tags:     parsedTags,
		json:     *jsonOut,
	}, nil
}

func parseStatsArgs(args []string) (statsOptions, error) {
	fs := newFlagSet("stats")
	jsonOut := fs.Bool("json", false, "Output machine-readable JSON")
	if err := parseFlags(fs, args, statsUsage); err != nil {
		return statsOptions{}, err
	}
	if fs.NArg() > 0 {
		return statsOptions{}, fmt.Errorf("unexpected argument(s) for stats: %s", strings.Join(fs.Args(), " "))
	}
	return statsOptions{json: *jsonOut}, nil
}

func parseResolveArgs(args []string) (resolveOptions, error) {
	fs := newFlagSet("resolve")
	reasonFlag := fs.String("reason", "", "Resolution reason")
	jsonOut := fs.Bool("json", false, "Output machine-readable JSON")
	if err := parseFlags(fs, args, resolveUsage); err != nil {
		return resolveOptions{}, err
	}
	if fs.NArg() < 1 {
		return resolveOptions{}, fmt.Errorf(resolveUsage)
	}
	if fs.NArg() > 2 {
		return resolveOptions{}, fmt.Errorf("too many arguments for resolve")
	}

	beadID := strings.TrimSpace(fs.Arg(0))
	if beadID == "" {
		return resolveOptions{}, fmt.Errorf("bead ID cannot be empty")
	}

	reason := strings.TrimSpace(*reasonFlag)
	if fs.NArg() == 2 {
		if reason != "" {
			return resolveOptions{}, fmt.Errorf("use either positional reason or --reason, not both")
		}
		reason = strings.TrimSpace(fs.Arg(1))
	}
	if reason == "" {
		reason = "resolved via CLI"
	}

	return resolveOptions{
		beadID: beadID,
		reason: reason,
		json:   *jsonOut,
	}, nil
}

func parseDoctorArgs(args []string) (doctorOptions, error) {
	fs := newFlagSet("doctor")
	jsonOut := fs.Bool("json", false, "Output machine-readable JSON")
	if err := parseFlags(fs, args, doctorUsage); err != nil {
		return doctorOptions{}, err
	}
	if fs.NArg() > 0 {
		return doctorOptions{}, fmt.Errorf("unexpected argument(s) for doctor: %s", strings.Join(fs.Args(), " "))
	}
	return doctorOptions{json: *jsonOut}, nil
}

func parseFlags(fs *flag.FlagSet, args []string, usageLine string) error {
	var parseErr bytes.Buffer
	fs.SetOutput(&parseErr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return errors.New(usageLine)
		}
		detail := strings.TrimSpace(parseErr.String())
		if detail != "" {
			first := strings.Split(detail, "\n")[0]
			return fmt.Errorf("%s (run with --help for details)", first)
		}
		return fmt.Errorf("invalid flags (run with --help for details)")
	}
	return nil
}

func newFlagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}

func parseTagValues(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}

	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			return nil, fmt.Errorf("tags must be comma-separated without empty values")
		}
		if !tagValuePattern.MatchString(tag) {
			return nil, fmt.Errorf("%q is not a valid tag (allowed: letters, numbers, '-', '_')", tag)
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result, nil
}

func filterByTags(in []beads.Bead, tags []string) []beads.Bead {
	if len(tags) == 0 {
		return in
	}
	normalizedNeedles := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalizedNeedles = append(normalizedNeedles, strings.ToLower(strings.TrimSpace(tag)))
	}

	filtered := make([]beads.Bead, 0, len(in))
	for _, bead := range in {
		if hasAllTags(bead.Tags, normalizedNeedles) {
			filtered = append(filtered, bead)
		}
	}
	return filtered
}

func hasAllTags(beadTags []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	available := map[string]struct{}{}
	for _, beadTag := range beadTags {
		normalized := strings.ToLower(strings.TrimSpace(beadTag))
		if normalized == "" {
			continue
		}
		available[normalized] = struct{}{}
	}

	for _, requiredTag := range required {
		if _, ok := available[requiredTag]; !ok {
			return false
		}
	}
	return true
}

func wantsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func exitWithError(message string, detail error, jsonOutput bool) {
	if jsonOutput {
		payload := map[string]interface{}{
			"error": message,
		}
		if detail != nil {
			payload["detail"] = detail.Error()
		}
		writeJSON(os.Stderr, payload)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Error: %s\n", message)
	os.Exit(1)
}

func writeJSON(out *os.File, payload interface{}) {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
