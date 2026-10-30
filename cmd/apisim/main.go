// Command apisim is the entry point for the apisim CLI.
//
// apisim is a chaos-testing proxy for third-party APIs. See docs/ for the
// full design and the scenario file reference.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apisim/internal/config"
	"apisim/internal/proxy"
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

func startCmd() *cobra.Command {
	var (
		port      string
		logLevel  string
		dbPath    string
		seed      int64
		dashboard bool
		failFast  bool
		ciMode    bool
	)
	cmd := &cobra.Command{
		Use:   "start <scenario.yaml>",
		Short: "Run the proxy/mock server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(args[0], startConfig{
				Port:      port,
				LogLevel:  logLevel,
				DBPath:    dbPath,
				Seed:      seed,
				Dashboard: dashboard,
				FailFast:  failFast,
				CI:        ciMode,
			})
		},
	}
	cmd.Flags().StringVar(&port, "port", "", "override listen port")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	cmd.Flags().StringVar(&dbPath, "db", "", "path to SQLite database")
	cmd.Flags().Int64Var(&seed, "seed", 0, "seed for reproducible randomness (overrides scenario)")
	cmd.Flags().BoolVar(&dashboard, "dashboard", false, "enable embedded dashboard (Phase 6)")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "exit on first config error")
	cmd.Flags().BoolVar(&ciMode, "ci", false, "machine-readable output; nonzero exit on config error")
	return cmd
}

type startConfig struct {
	Port      string
	LogLevel  string
	DBPath    string
	Seed      int64
	Dashboard bool
	FailFast  bool
	CI        bool
}

func runStart(scenarioPath string, cfg startConfig) error {
	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		return fmt.Errorf("read scenario: %w", err)
	}
	scen, err := config.Load(data)
	if err != nil {
		if cfg.CI {
			fmt.Printf("CONFIG_ERROR:%v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "invalid scenario: %v\n", err)
		}
		return err
	}

	// CLI overrides.
	if cfg.Port != "" {
		scen.Listen = cfg.Port
	}
	if cfg.Seed != 0 {
		scen.Seed = cfg.Seed
	}

	listen := scen.Listen
	if listen == "" {
		listen = ":8080"
	}

	router, err := proxy.New(scen)
	if err != nil {
		return fmt.Errorf("build proxy: %w", err)
	}

	srv := &http.Server{
		Addr:    listen,
		Handler: loggingMiddleware(router),
	}

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	fmt.Fprintf(os.Stderr, "apisim listening on %s (scenario %q)\n", listen, scen.Name)

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <scenario.yaml>",
		Short: "Validate a scenario file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read scenario: %w", err)
			}
			if _, err := config.Load(data); err != nil {
				fmt.Fprintf(os.Stderr, "invalid: %v\n", err)
				return err
			}
			fmt.Println("valid")
			return nil
		},
	}
}

// loggingMiddleware wraps the handler with structured request logging.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		dur := time.Since(start)
		fmt.Printf("[%s] %s %s -> %d (%v)\n",
			time.Now().Format(time.RFC3339), r.Method, r.URL.Path, ww.status, dur)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *responseWriter) WriteHeader(code int) {
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
