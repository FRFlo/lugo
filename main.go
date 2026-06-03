package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/coalaura/lugo/lsp"
)

// Version specifies the current build version of the Lugo binary.
var Version = "dev"

func main() {
	ciFlag := flag.String("ci", "", "Path to CI configuration JSON file")
	flag.Parse()

	tel, err := lsp.InitTelemetry(Version)
	if err != nil {
		// We don't want to crash the LSP just because telemetry failed to init
		fmt.Fprintf(os.Stderr, "Telemetry init failed: %v\n", err)
	} else if tel != nil {
		defer tel.Close()
	}

	defer func() {
		if r := recover(); r != nil {
			lsp.CapturePanic(r, "main")
			if tel != nil {
				tel.Close() // Flush events
			}
			panic(r) // Re-panic to retain original behavior
		}
	}()

	server := lsp.NewServer(Version)

	if *ciFlag != "" {
		os.Exit(server.RunCI(*ciFlag))
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
