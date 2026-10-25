package main

import (
	"context"
	"io"

	"github.com/aalvaropc/upkeep/internal/stub"
)

func serve(_ context.Context, _ []string, _, _ io.Writer, _ []string) error {
	return stub.Err("serve")
}
