package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	days := flag.Int("days", 1, "number of days to fetch (starting today)")
	future := flag.Bool("future", false, "only show meetings that haven't ended yet")
	text := flag.Bool("text", false, "show colored text output instead of JSON")
	tui := flag.Bool("tui", false, "interactive TUI with meeting list and detail pane")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: qcal [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Fetch calendar meetings from multiple sources and display them.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	sources, err := buildSources(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	meetings, warnings, err := fetchAllSources(sources, *days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", w)
	}

	if *future {
		meetings = filterFuture(meetings)
	}

	colors := BuildSourceColors(cfg.Sources)
	ensureMeetingColors(meetings, colors)

	if *tui {
		if err := runTUI(meetings, colors); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *text {
		printText(meetings, colors)
		return
	}

	output, err := json.MarshalIndent(meetings, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(output))
}
