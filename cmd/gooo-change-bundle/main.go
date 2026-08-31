package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/kimjooyoon/gooo-change-bundle/internal/bundle"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: gooo-change-bundle digest|materialize [flags]")
	}
	switch os.Args[1] {
	case "digest":
		digestCommand(os.Args[2:])
	case "materialize", "create":
		materializeCommand(os.Args[2:])
	default:
		fatal("usage: gooo-change-bundle digest|materialize [flags]")
	}
}

func digestCommand(args []string) {
	flags := flag.NewFlagSet("digest", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("source-root", "", "source tree directory")
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if *root == "" {
		fatal("--source-root is required")
	}
	digest, err := bundle.ComputeSourceDigest(*root)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Println(digest)
}

func materializeCommand(args []string) {
	flags := flag.NewFlagSet("materialize", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := bundle.Options{}
	flags.StringVar(&options.SourceRoot, "source-root", "", "exact source tree directory")
	flags.StringVar(&options.SourceDigest, "source-digest", "", "exact expected source tree digest")
	flags.StringVar(&options.ProposalPath, "proposal", "", "approved proposal JSON")
	flags.StringVar(&options.AuthorityPath, "authority", "", "authority receipt JSON")
	flags.StringVar(&options.IntentPath, "intent", "", ".gooo change intent")
	flags.StringVar(&options.ContractPath, "contract", "", "fixed denominator contract JSON")
	flags.StringVar(&options.OutputDir, "out", "", "empty caller-owned output directory")
	flags.StringVar(&options.ObservationPath, "observation", "", "optional observation JSON")
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if options.SourceRoot == "" || options.IntentPath == "" || options.ContractPath == "" || options.OutputDir == "" {
		fatal("--source-root, --intent, --contract, and --out are required")
	}
	result, err := bundle.Run(options)
	if err != nil {
		fatal(err.Error())
	}
	summary, err := json.Marshal(struct {
		Decision      string `json:"decision"`
		ChangedPaths  int    `json:"changed_paths"`
		BundleFiles   int    `json:"bundle_files"`
		ReplayMismatch int   `json:"replay_mismatches"`
	}{Decision: result.Manifest.Decision, ChangedPaths: len(result.Manifest.ChangedPaths), BundleFiles: len(result.Manifest.Artifacts), ReplayMismatch: result.Manifest.Metrics.ReplayMismatches})
	if err != nil {
		fatal(err.Error())
	}
	fmt.Println(string(summary))
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

