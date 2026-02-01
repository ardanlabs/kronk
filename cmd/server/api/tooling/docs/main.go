package main

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/api/tooling/docs/manual"
	"github.com/ardanlabs/kronk/cmd/server/api/tooling/docs/sdk/examples"
	"github.com/ardanlabs/kronk/cmd/server/api/tooling/docs/sdk/gofmt"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	if err := gofmt.Run(); err != nil {
		return err
	}

	if err := examples.Run(); err != nil {
		return err
	}

	if err := manual.Run(); err != nil {
		return err
	}

	return nil
}
