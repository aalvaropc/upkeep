package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/aalvaropc/upkeep/internal/buildinfo"
	"github.com/aalvaropc/upkeep/internal/stub"
)

const usage = `usage: upkeep <command>

commands:
  serve     start the dashboard and API
  scenario  build the demo repository with its nine pull requests
  run       work the open pull requests of a repository
  demo      serve the hosted demo with isolated sessions
  record    run the demo scenario live and record every external call
  eval      run the demo scenario and write outcome metrics
  version   print the version
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Environ())
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, buildinfo.String())
		return 0
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "serve":
		err = serve(ctx, args[1:], stdout, stderr, environ)
	case "scenario":
		err = scenarioCommand(ctx, args[1:], stdout, stderr)
	case "run", "demo", "record", "eval":
		err = stub.Err(args[0])
	default:
		fmt.Fprintf(stderr, "upkeep: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "upkeep: %v\n", err)
		return 1
	}
	return 0
}
