package handlers

import (
	"testing"
)

func TestRuntimeMetadataUsesReleaseEnvironment(t *testing.T) {
	t.Setenv("MAGICPODCAST_RELEASE_ID", "20260712T000000Z-test")
	t.Setenv("MAGICPODCAST_FRONTEND_BUILD_ID", "frontend-build-1")
	t.Setenv("MAGICPODCAST_SERVER_MODE", "release")

	metadata := runtimeMetadata()
	if metadata["release_id"] != "20260712T000000Z-test" {
		t.Fatalf("release_id = %v", metadata["release_id"])
	}
	if metadata["frontend_build_id"] != "frontend-build-1" {
		t.Fatalf("frontend_build_id = %v", metadata["frontend_build_id"])
	}
	if metadata["build_mode"] != "release" {
		t.Fatalf("build_mode = %v", metadata["build_mode"])
	}
}

func TestRuntimeMetadataDefaultsToUnknown(t *testing.T) {
	t.Setenv("MAGICPODCAST_RELEASE_ID", "")
	t.Setenv("MAGICPODCAST_FRONTEND_BUILD_ID", "")
	t.Setenv("MAGICPODCAST_SERVER_MODE", "")

	metadata := runtimeMetadata()
	for _, key := range []string{"release_id", "frontend_build_id", "build_mode"} {
		if metadata[key] != "unknown" {
			t.Fatalf("%s = %v, want unknown", key, metadata[key])
		}
	}
}
