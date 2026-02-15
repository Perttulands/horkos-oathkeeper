package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/perttulands/oathkeeper/pkg/beads"
	"github.com/perttulands/oathkeeper/pkg/config"
)

const usage = `Oathkeeper — Beads-native commitment guard

Usage:
  oathkeeper <command> [flags]

Commands:
  serve    Start the HTTP server and daemon
  scan     Batch scan a transcript file
  list     List open oathkeeper beads
  stats    Show commitment statistics
  resolve  Resolve a commitment bead
  doctor   Run health checks

Flags:
  --config PATH  Config file (default: ~/.config/oathkeeper/oathkeeper.toml)
  --help         Show this help
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	cmd := os.Args[1]

	// Global flags
	configPath := ""
	for i, arg := range os.Args[2:] {
		if arg == "--config" && i+1 < len(os.Args[2:])-1 {
			configPath = os.Args[2:][i+1]
		}
	}

	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		fmt.Print(usage)
		return
	}

	switch cmd {
	case "serve":
		startServer(configPath)
	case "scan":
		runScan(os.Args[2:])
	case "list":
		runList(configPath)
	case "stats":
		runStats(configPath)
	case "resolve":
		runResolve(configPath, os.Args[2:])
	case "doctor":
		runDoctor(configPath)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", cmd, usage)
		os.Exit(1)
	}
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
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: oathkeeper scan <file>")
		os.Exit(1)
	}

	file := fs.Arg(0)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "file not found: %s\n", file)
		os.Exit(1)
	}

	fmt.Printf("Scanning %s...\n", file)
	fmt.Println("Scan not yet wired to batch scanner.")
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

	for _, b := range list {
		tags := strings.Join(b.Tags, ", ")
		fmt.Printf("  %s  %s  [%s]\n", b.ID, b.Title, tags)
	}
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
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	fs.Parse(args)

	// Filter out --config and its value from remaining args
	remaining := fs.Args()
	var filtered []string
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "--config" {
			i++ // skip value
			continue
		}
		filtered = append(filtered, remaining[i])
	}

	if len(filtered) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: oathkeeper resolve <bead-id> [reason]")
		os.Exit(1)
	}

	beadID := filtered[0]
	reason := "resolved via CLI"
	if len(filtered) > 1 {
		reason = strings.Join(filtered[1:], " ")
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
	fmt.Println("Oathkeeper Health Check")
	fmt.Println("=======================")

	// Check br command
	store := beads.NewBeadStore(cfg.Verification.BeadsCommand)
	_, err := store.List(beads.Filter{Status: "open"})
	if err != nil {
		fmt.Printf("  br CLI (%s): FAIL (%v)\n", cfg.Verification.BeadsCommand, err)
	} else {
		fmt.Printf("  br CLI (%s): OK\n", cfg.Verification.BeadsCommand)
	}

	fmt.Printf("  Config: %s\n", config.ExpandPath(configPath))
	fmt.Printf("  Grace period: %v\n", cfg.GracePeriodDuration())
}
