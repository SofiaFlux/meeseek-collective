package main

import (
	"context"
	"os"
)

func run(context.Context) error {
	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		os.Exit(1)
	}
}
