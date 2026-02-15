package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

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
	configPath, subArgs := extractConfigFlag(os.Args[2:])

	switch cmd {
	case "serve":
		startServer(configPath)
	case "scan":
		runScan(subArgs)
	case "list":
		runList(configPath)
	case "stats":
		runStats(configPath)
	case "resolve":
		runResolve(configPath, subArgs)
	case "doctor":
		runDoctor(configPath)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", cmd, usage)
		os.Exit(1)
	}
}

// extractConfigFlag pulls --config VALUE from args, returning the config path
// and remaining args with --config and its value removed.
func extractConfigFlag(args []string) (string, []string) {
	configPath := ""
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			configPath = args[i+1]
			i++ // skip value
			continue
		}
		remaining = append(remaining, args[i])
	}
	return configPath, remaining
}

func loadConfig(configPath string) *config.Config {
	if configPath == "" {
		configPath = config.ExpandPath(config.DefaultConfigPath())
	} else {
		configPath = config.ExpandPath(configPath)
	}
	return config.LoadOrDefault(configPath)
}

func runScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	format := fs.String("format", "text", "Output format: text or json")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: oathkeeper scan <file> [--format text|json]")
		os.Exit(1)
	}

	file := fs.Arg(0)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: file not found: %s\n", file)
		os.Exit(1)
	}

	results, err := scanner.ScanFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		fmt.Print(scanner.FormatScanResultsJSON(results))
	default:
		fmt.Print(scanner.FormatScanResults(results))
	}
}

func runList(configPath string) {
	cfg := loadConfig(configPath)
	store := beads.NewBeadStore(cfg.Verification.BeadsCommand)

	list, err := store.List(beads.Filter{Status: "open"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(list) == 0 {
		fmt.Println("No open oathkeeper commitments.")
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
	fmt.Printf("\n%d open commitment(s)\n", len(list))
}

func runStats(configPath string) {
	cfg := loadConfig(configPath)
	store := beads.NewBeadStore(cfg.Verification.BeadsCommand)

	list, err := store.List(beads.Filter{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

func runResolve(configPath string, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: oathkeeper resolve <bead-id> [reason]")
		os.Exit(1)
	}

	beadID := args[0]
	reason := "resolved via CLI"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	cfg := loadConfig(configPath)
	store := beads.NewBeadStore(cfg.Verification.BeadsCommand)

	if err := store.Resolve(beadID, reason); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Resolved %s: %s\n", beadID, reason)
}

func runDoctor(configPath string) {
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

	fmt.Println(doctor.FormatReport(results))
}
