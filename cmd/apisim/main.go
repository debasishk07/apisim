// Command apisim is the entry point for the apisim CLI.
//
// apisim is a chaos-testing proxy for third-party APIs. See docs/ for the
// full design and the scenario file reference.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

// buildDate is injected at build time via -ldflags.
var buildDate = ""

// gitCommit is injected at build time via -ldflags.
var gitCommit = ""

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apisim",
		Short: "apisim — chaos testing for third-party APIs",
		Long: `apisim sits between your app and external APIs (Stripe, Twilio,
OpenAI, etc.) and injects realistic failures so developers can verify their
app handles outages, slowness, bad data, and webhook problems.

Use 'apisim start <scenario.yaml>' to run the proxy/mock server.`,
		Version: versionString(),
	}

	cmd.AddCommand(versionCmd())
	cmd.AddCommand(startCmd())
	cmd.AddCommand(validateCmd())

	return cmd
}

func versionString() string {
	if buildDate != "" || gitCommit != "" {
		return fmt.Sprintf("%s (build %s, commit %s)", version, buildDate, gitCommit)
	}
	return version
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the apisim version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(versionString())
		},
	}
}

// startCmd is a stub for Phase 1.
func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <scenario.yaml>",
		Short: "Run the proxy/mock server (Phase 1)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("start not yet implemented — see Phase 1")
		},
	}
}

// validateCmd is a stub for Phase 1.
func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <scenario.yaml>",
		Short: "Validate a scenario file (Phase 1)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("validate not yet implemented — see Phase 1")
		},
	}
}
