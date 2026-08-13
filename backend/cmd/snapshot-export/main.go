package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"magicpodcast/internal/dataprofile"
)

const exportConfirmation = "I_AUTHORIZE_READ_ONLY_PRODUCTION_SNAPSHOT_EXPORT"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runContext(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "snapshot-export: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	return runContext(context.Background(), args)
}

func runContext(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("snapshot-export", flag.ContinueOnError)
	source := flags.String("source", "", "production SQLite path")
	output := flags.String("output", "", "empty secure staging directory")
	snapshotID := flags.String("id", dataprofile.DefaultSnapshotID(time.Now()), "snapshot ID")
	capturedAt := flags.String("captured-at", time.Now().UTC().Format(time.RFC3339), "capture time")
	confirmation := flags.String("confirm", "", "explicit production read authorization")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *confirmation != exportConfirmation {
		return fmt.Errorf("read-only production export requires explicit confirmation")
	}
	if *source == "" || *output == "" {
		return fmt.Errorf("--source and --output are required")
	}
	result, err := dataprofile.ExportSanitizedSnapshotContext(
		ctx,
		*source,
		*output,
		*snapshotID,
		*capturedAt,
	)
	if err != nil {
		return err
	}
	fmt.Printf("snapshot_id=%s\n", result.Manifest.ID)
	fmt.Printf("captured_at=%s\n", result.Manifest.CapturedAt)
	fmt.Printf("schema=%d\n", result.Manifest.SchemaVersion)
	fmt.Printf("sanitizer_version=%s\n", result.Manifest.SanitizerVersion)
	fmt.Printf("sha256=%s\n", result.Manifest.SHA256)
	fmt.Printf("staging_directory=%s\n", *output)
	return nil
}
