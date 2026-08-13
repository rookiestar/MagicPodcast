package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"magicpodcast/internal/dataprofile"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "data-profile: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("data-profile", flag.ContinueOnError)
	projectDir := flags.String("project-dir", "", "MagicPodcast repository root")
	profileHome := flags.String("home", "", "managed profile directory")
	port := flags.Int("port", defaultPort(), "managed backend loopback port")
	timeout := flags.Duration("timeout", 30*time.Second, "backend readiness timeout")
	asJSON := flags.Bool("json", false, "print JSON status")
	transferDir := flags.String("transfer-dir", "", "local transfer staging directory (test/manual handoff)")
	refreshConfirmation := flags.String("confirm-refresh", "", "explicit snapshot refresh authorization")
	retention := flags.Int("keep", 3, "number of recent snapshots to retain")
	if err := flags.Parse(args); err != nil {
		return err
	}
	remaining := flags.Args()
	if len(remaining) == 0 {
		return usageError()
	}

	if *projectDir == "" {
		workingDir, err := os.Getwd()
		if err != nil {
			return err
		}
		*projectDir = filepath.Clean(filepath.Join(workingDir, ".."))
	}
	if *profileHome == "" {
		defaultHome, err := dataprofile.DefaultProfileHome()
		if err != nil {
			return err
		}
		*profileHome = defaultHome
	}
	controller := dataprofile.Controller{
		ProjectDir:  *projectDir,
		ProfileHome: *profileHome,
		Port:        *port,
		Timeout:     *timeout,
	}
	ctx := context.Background()

	var (
		status dataprofile.PublicStatus
		err    error
	)
	switch remaining[0] {
	case "status":
		if len(remaining) != 1 {
			return usageError()
		}
		status, err = controller.Status(ctx)
	case "use":
		if len(remaining) < 2 || len(remaining) > 3 {
			return usageError()
		}
		switch remaining[1] {
		case "fixture":
			if len(remaining) > 3 {
				return usageError()
			}
			scenario := dataprofile.DefaultFixtureScenario
			if len(remaining) == 3 {
				scenario = remaining[2]
			}
			status, err = controller.UseFixtureScenario(ctx, scenario)
		case "snapshot":
			selector := "latest"
			if len(remaining) == 3 {
				selector = remaining[2]
			}
			status, err = controller.UseSnapshot(ctx, selector)
		case "production":
			return fmt.Errorf("production profile cannot be selected by the local data-profile command")
		default:
			return fmt.Errorf("unsupported profile %q", remaining[1])
		}
	case "snapshot":
		if len(remaining) != 2 || remaining[1] != "refresh" {
			return usageError()
		}
		var transfer dataprofile.Transfer
		source := *transferDir
		if *transferDir != "" {
			transfer = dataprofile.LocalDirectoryTransfer{}
		} else if adapter := strings.TrimSpace(os.Getenv("MAGICPODCAST_SNAPSHOT_TRANSFER_ADAPTER")); adapter != "" {
			if !filepath.IsAbs(adapter) {
				return fmt.Errorf("snapshot transfer adapter must be an absolute executable path")
			}
			transfer = dataprofile.CommandTransfer{Command: []string{adapter}}
			source = "configured-adapter"
		} else {
			return fmt.Errorf("snapshot refresh requires MAGICPODCAST_SNAPSHOT_TRANSFER_ADAPTER or a prepared --transfer-dir handoff")
		}
		snapshot, refreshErr := controller.RefreshSnapshot(ctx, dataprofile.RefreshRequest{
			Source:       source,
			Confirmation: *refreshConfirmation,
			Keep:         *retention,
		}, transfer)
		if refreshErr != nil {
			return refreshErr
		}
		status = dataprofile.PublicStatus{
			Managed:            false,
			Profile:            "not-switched",
			Ready:              false,
			SchemaVersion:      snapshot.Manifest.SchemaVersion,
			SnapshotID:         snapshot.ID,
			SnapshotCapturedAt: snapshot.Manifest.CapturedAt,
		}
	default:
		return usageError()
	}
	if err != nil {
		return err
	}
	return printStatus(status, *asJSON)
}

func defaultPort() int {
	if value := os.Getenv("MAGICPODCAST_DATA_PROFILE_PORT"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return 8080
}

func printStatus(status dataprofile.PublicStatus, asJSON bool) error {
	if asJSON {
		data, err := json.Marshal(status)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("managed=%t\n", status.Managed)
	fmt.Printf("profile=%s\n", status.Profile)
	fmt.Printf("ready=%t\n", status.Ready)
	if status.SchemaVersion != 0 {
		fmt.Printf("schema=%d\n", status.SchemaVersion)
	}
	if status.FixtureVersion != "" {
		fmt.Printf("fixture_version=%s\n", status.FixtureVersion)
		fmt.Printf("fixture_scenario=%s\n", status.FixtureScenario)
		fmt.Printf("fixture_anchor_at=%s\n", status.FixtureAnchorAt)
	}
	if status.SnapshotID != "" {
		fmt.Printf("snapshot_id=%s\n", status.SnapshotID)
		fmt.Printf("snapshot_captured_at=%s\n", status.SnapshotCapturedAt)
	}
	if status.InstanceID != "" {
		fmt.Printf("instance_id=%s\n", status.InstanceID)
	}
	return nil
}

func usageError() error {
	return fmt.Errorf("usage: data-profile [flags] status | use fixture [scenario] | use snapshot [latest|ID] | snapshot refresh")
}
