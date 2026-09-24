package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/FRFlo/lugo/lsp"
)

// Version specifies the current build version of the Lugo binary.
var Version = "dev"

func main() {
	ciFlag := flag.String("ci", "", "Path to CI configuration JSON file")
	flag.Parse()

	tel, err := lsp.InitTelemetry(Version)
	if err != nil {
		// We don't want to crash the LSP just because telemetry failed to init
		// Initialization errors can contain a user-configured journal path, so do
		// not expose their raw text on the process transport.
		fmt.Fprintln(os.Stderr, "Telemetry initialization failed")
	} else if tel != nil {
		defer tel.Close()
	}

	defer func() {
		if r := recover(); r != nil {
			lsp.CapturePanic(r, "main")
			// Persist the local envelope before re-panicking. The deferred Close
			// below also gives PostHog its configured bounded shutdown flush.
			lsp.FlushTelemetry()
			panic(r) // Re-panic to retain original behavior
		}
	}()

	server := lsp.NewServer(Version)

	if *ciFlag != "" {
		// os.Exit skips defers, so CI must explicitly flush and close telemetry.
		code := server.RunCI(*ciFlag)
		if tel != nil {
			_ = tel.Flush()
			tel.Close()
		}
		os.Exit(code)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()

		_ = os.Stdin.Close()
	}()

	err = server.Start()
	if err != nil {
		panic(err)
	}
}
