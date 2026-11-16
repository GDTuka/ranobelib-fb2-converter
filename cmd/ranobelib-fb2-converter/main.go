package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"ranobelib.parser/internal/config"
	"ranobelib.parser/internal/converter"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("ranobelib-fb2-converter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to JSON configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("-config is required")
	}

	appConfig, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	appConverter := converter.NewAPI(httpClient)
	outputPath, err := appConverter.Convert(ctx, appConfig)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "FB2 file successfully created: %s\n", outputPath)
	return nil
}
