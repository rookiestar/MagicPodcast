package processing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

func TestIMAManualImportBridgePublishesDeterministicRestrictedPackage(t *testing.T) {
	request := validIMAManualImportRequest()
	firstRoot := filepath.Join(t.TempDir(), "first")
	first, err := NewIMAManualImportBridge(firstRoot)
	require.NoError(t, err)
	require.Equal(t, DeliveryModeManualImport, first.DeliveryMode())

	receipt, err := first.Deliver(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, models.DeliveryStatusPending, receipt.Status)
	require.Regexp(
		t,
		"^ima-manual-import:"+request.DeliveryKey+":[a-f0-9]{64}$",
		receipt.RemoteRef,
	)
	require.Empty(t, receipt.PublicURL)
	require.NotContains(t, receipt.RemoteRef, firstRoot)

	packagePath := filepath.Join(first.root, "packages", request.DeliveryKey)
	expectedNames := []string{"IMPORT.md", "knowledge.md", "manifest.json", "metadata.json"}
	entries, err := os.ReadDir(packagePath)
	require.NoError(t, err)
	require.Len(t, entries, len(expectedNames))
	for index, entry := range entries {
		require.Equal(t, expectedNames[index], entry.Name())
		info, infoErr := entry.Info()
		require.NoError(t, infoErr)
		require.True(t, info.Mode().IsRegular())
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	for _, directory := range []string{first.root, filepath.Join(first.root, "packages"), packagePath} {
		directoryInfo, statErr := os.Stat(directory)
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o700), directoryInfo.Mode().Perm())
	}

	firstFiles := readIMAPackageFiles(t, packagePath)
	require.Equal(
		t,
		"ima-manual-import:"+request.DeliveryKey+":"+digestBytes(firstFiles["manifest.json"]),
		receipt.RemoteRef,
	)
	var manifest imaPackageManifest
	require.NoError(t, json.Unmarshal(firstFiles["manifest.json"], &manifest))
	require.Equal(t, IMAManualImportPackageSchema+".manifest", manifest.Schema)
	require.Equal(t, IMAManualImportSchemaVersion, manifest.Version)
	require.Equal(t, DeliveryModeManualImport, manifest.DeliveryMode)
	require.Equal(t, request.DeliveryKey, manifest.PackageID)
	require.Equal(t, "SHA-256", manifest.ChecksumAlgorithm)
	require.Len(t, manifest.Files, 3)
	for _, file := range manifest.Files {
		require.Equal(t, digestBytes(firstFiles[file.Path]), file.SHA256)
		require.Equal(t, len(firstFiles[file.Path]), file.Size)
	}

	var metadata imaPackageMetadata
	require.NoError(t, json.Unmarshal(firstFiles["metadata.json"], &metadata))
	require.Equal(t, IMAManualImportPackageSchema+".metadata", metadata.Schema)
	require.Equal(t, IMAManualImportSchemaVersion, metadata.Version)
	require.Equal(t, request.Package.EpisodeTitle, metadata.Episode.Title)
	require.Equal(t, request.Package.PodcastTitle, metadata.Episode.Podcast)
	require.Equal(t, request.ArtifactSetID, metadata.Artifact.ArtifactSetID)
	require.Equal(t, request.Package.ManifestSHA256, metadata.Artifact.ManifestSHA256)
	require.Contains(t, string(firstFiles["knowledge.md"]), "规范逐字稿")
	require.Contains(t, string(firstFiles["knowledge.md"]), "单集纪要")
	require.Contains(t, string(firstFiles["knowledge.md"]), "Show Notes")

	allContent := string(firstFiles["knowledge.md"]) +
		string(firstFiles["metadata.json"]) +
		string(firstFiles["manifest.json"]) +
		string(firstFiles["IMPORT.md"])
	for _, forbidden := range []string{
		".mp3",
		"PRIVATE-NOTE",
		"file_token",
		"minute_token",
		"/Users/",
		firstRoot,
	} {
		require.NotContains(t, allContent, forbidden)
	}

	repeatedRequest := request
	repeatedRequest.Package.Sources = cloneStringMap(request.Package.Sources)
	repeatedRequest.Package.EpisodeTitle = "后续被修改的标题"
	repeatedRequest.Package.ShowNotes = "后续被修改的 Show Notes"
	repeatedRequest.Package.SourceURL = "https://example.com/episodes/9?revision=2"
	repeatedRequest.Package.Sources["episode"] = repeatedRequest.Package.SourceURL
	secondReceipt, err := first.Deliver(context.Background(), repeatedRequest)
	require.NoError(t, err)
	require.Equal(t, receipt, secondReceipt)
	require.Equal(t, firstFiles, readIMAPackageFiles(t, packagePath))
	packageEntries, err := os.ReadDir(filepath.Join(first.root, "packages"))
	require.NoError(t, err)
	require.Len(t, packageEntries, 1)

	second, err := NewIMAManualImportBridge(filepath.Join(t.TempDir(), "second"))
	require.NoError(t, err)
	_, err = second.Deliver(context.Background(), request)
	require.NoError(t, err)
	require.Equal(
		t,
		firstFiles,
		readIMAPackageFiles(t, filepath.Join(second.root, "packages", request.DeliveryKey)),
	)
}

func TestIMAManualImportBridgeSchemaUpgradeUsesNewPackageIdentity(t *testing.T) {
	request := validIMAManualImportRequest()
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)
	_, err = bridge.Deliver(context.Background(), request)
	require.NoError(t, err)

	legacyVersion := "0.9.0"
	legacyPath := filepath.Join(bridge.root, "packages", request.DeliveryKey)
	legacyFiles := readIMAPackageFiles(t, legacyPath)
	var metadata imaPackageMetadata
	require.NoError(t, json.Unmarshal(legacyFiles["metadata.json"], &metadata))
	metadata.Version = legacyVersion
	legacyFiles["metadata.json"], err = marshalStableJSON(metadata)
	require.NoError(t, err)
	legacyFiles["knowledge.md"] = []byte(strings.ReplaceAll(
		string(legacyFiles["knowledge.md"]),
		IMAManualImportSchemaVersion,
		legacyVersion,
	))

	var manifest imaPackageManifest
	require.NoError(t, json.Unmarshal(legacyFiles["manifest.json"], &manifest))
	manifest.Version = legacyVersion
	for index := range manifest.Files {
		content := legacyFiles[manifest.Files[index].Path]
		manifest.Files[index].Size = len(content)
		manifest.Files[index].SHA256 = digestBytes(content)
	}
	legacyFiles["manifest.json"], err = marshalStableJSON(manifest)
	require.NoError(t, err)
	for name, content := range legacyFiles {
		require.NoError(t, os.WriteFile(filepath.Join(legacyPath, name), content, 0o600))
	}

	_, err = bridge.Deliver(context.Background(), request)
	require.Error(t, err)
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "invalid_ima_manual_import_package", adapterErr.ErrorCode)

	upgradedRequest := request
	upgradedRequest.DeliveryKey = strings.Repeat("b", 64)
	_, err = bridge.Deliver(context.Background(), upgradedRequest)
	require.NoError(t, err)
	upgradedFiles := readIMAPackageFiles(
		t,
		filepath.Join(bridge.root, "packages", upgradedRequest.DeliveryKey),
	)
	require.NoError(t, json.Unmarshal(upgradedFiles["manifest.json"], &manifest))
	require.Equal(t, IMAManualImportSchemaVersion, manifest.Version)
	require.Equal(t, upgradedRequest.DeliveryKey, manifest.PackageID)
}

func TestIMAManualImportBridgeRejectsFilesystemRoot(t *testing.T) {
	_, err := NewIMAManualImportBridge(string(os.PathSeparator))
	require.ErrorContains(t, err, "cannot be the filesystem root")
}

func TestIMAManualImportBridgeAllowsMissingPublicSource(t *testing.T) {
	request := validIMAManualImportRequest()
	request.Package.SourceURL = ""
	delete(request.Package.Sources, "episode")
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)

	_, err = bridge.Deliver(context.Background(), request)
	require.NoError(t, err)
	files := readIMAPackageFiles(
		t,
		filepath.Join(bridge.root, "packages", request.DeliveryKey),
	)
	var metadata imaPackageMetadata
	require.NoError(t, json.Unmarshal(files["metadata.json"], &metadata))
	require.Empty(t, metadata.Episode.SourceURL)
	require.Contains(t, string(files["knowledge.md"]), "来源：未提供公开链接")
}

func TestIMAManualImportBridgePreservesRestrictedFeishuTraceRefs(t *testing.T) {
	request := validIMAManualImportRequest()
	request.Package.Sources = map[string]string{
		"episode":           "https://example.com/episodes/9",
		"transcription":     "feishu-minutes",
		"feishu_drive_ref":  "sha256:" + strings.Repeat("a", 64),
		"feishu_minute_ref": "sha256:" + strings.Repeat("b", 64),
	}
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)

	_, err = bridge.Deliver(context.Background(), request)
	require.NoError(t, err)
	files := readIMAPackageFiles(
		t,
		filepath.Join(bridge.root, "packages", request.DeliveryKey),
	)
	var metadata imaPackageMetadata
	require.NoError(t, json.Unmarshal(files["metadata.json"], &metadata))
	require.Equal(t, request.Package.Sources, metadata.Sources)
	for _, content := range files {
		require.NotContains(t, string(content), "file_token")
		require.NotContains(t, string(content), "minute_token")
	}
}

func TestIMAManualImportBridgeMarksMissingPublicationDateUnavailable(t *testing.T) {
	request := validIMAManualImportRequest()
	request.Package.PublishedAt = time.Time{}
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)

	_, err = bridge.Deliver(context.Background(), request)
	require.NoError(t, err)
	files := readIMAPackageFiles(
		t,
		filepath.Join(bridge.root, "packages", request.DeliveryKey),
	)
	var metadata imaPackageMetadata
	require.NoError(t, json.Unmarshal(files["metadata.json"], &metadata))
	require.Nil(t, metadata.Episode.PublishedAt)
	require.Contains(t, string(files["knowledge.md"]), "- 发布日期：不可用")
	require.NotContains(t, string(files["knowledge.md"]), "0001-01-01")
}

func TestIMAManualImportBridgeRejectsIncompleteArtifacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DeliveryRequest)
	}{
		{"artifact set", func(request *DeliveryRequest) { request.ArtifactSetID = 0 }},
		{"delivery key", func(request *DeliveryRequest) { request.DeliveryKey = "not-a-sha" }},
		{"run", func(request *DeliveryRequest) { request.Package.RunID = 0 }},
		{"episode", func(request *DeliveryRequest) { request.Package.EpisodeID = 0 }},
		{"title", func(request *DeliveryRequest) { request.Package.EpisodeTitle = "" }},
		{"podcast", func(request *DeliveryRequest) { request.Package.PodcastTitle = "" }},
		{"pipeline", func(request *DeliveryRequest) { request.Package.PipelineVersion = "" }},
		{"artifact time", func(request *DeliveryRequest) { request.Package.ArtifactGeneratedAt = time.Time{} }},
		{"manifest checksum", func(request *DeliveryRequest) { request.Package.ManifestSHA256 = "" }},
		{"transcript", func(request *DeliveryRequest) { request.Package.Transcript = "" }},
		{"notes", func(request *DeliveryRequest) { request.Package.EpisodeNotes = "" }},
		{"transcript mismatch", func(request *DeliveryRequest) { request.Package.Transcript += "tampered" }},
		{"notes mismatch", func(request *DeliveryRequest) { request.Package.EpisodeNotes += "tampered" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validIMAManualImportRequest()
			test.mutate(&request)
			bridge, err := NewIMAManualImportBridge(t.TempDir())
			require.NoError(t, err)
			_, err = bridge.Deliver(context.Background(), request)
			require.Error(t, err)
			var adapterErr *AdapterError
			require.ErrorAs(t, err, &adapterErr)
			require.Equal(t, "invalid_ima_manual_import_package", adapterErr.ErrorCode)
			entries, readErr := os.ReadDir(bridge.root)
			require.NoError(t, readErr)
			require.Empty(t, entries)
		})
	}
}

func TestIMAManualImportBridgeRejectsTraversalSymlinksAndSensitiveSources(t *testing.T) {
	t.Run("destination traversal", func(t *testing.T) {
		request := validIMAManualImportRequest()
		request.Destination = "../../escape"
		root := t.TempDir()
		bridge, err := NewIMAManualImportBridge(root)
		require.NoError(t, err)
		_, err = bridge.Deliver(context.Background(), request)
		require.Error(t, err)
		require.NoDirExists(t, filepath.Join(root, "packages"))
	})

	t.Run("root symlink", func(t *testing.T) {
		parent := t.TempDir()
		realRoot := filepath.Join(parent, "real")
		require.NoError(t, os.Mkdir(realRoot, 0o700))
		linkRoot := filepath.Join(parent, "link")
		require.NoError(t, os.Symlink(realRoot, linkRoot))
		_, err := NewIMAManualImportBridge(linkRoot)
		require.Error(t, err)
	})

	t.Run("package symlink", func(t *testing.T) {
		request := validIMAManualImportRequest()
		bridge, err := NewIMAManualImportBridge(t.TempDir())
		require.NoError(t, err)
		packagesRoot := filepath.Join(bridge.root, "packages")
		require.NoError(t, os.Mkdir(packagesRoot, 0o700))
		outside := t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(packagesRoot, request.DeliveryKey)))
		_, err = bridge.Deliver(context.Background(), request)
		require.Error(t, err)
		outsideEntries, readErr := os.ReadDir(outside)
		require.NoError(t, readErr)
		require.Empty(t, outsideEntries)
	})

	tests := []struct {
		name   string
		mutate func(*DeliveryRequest)
	}{
		{"feishu token key", func(request *DeliveryRequest) {
			request.Package.Sources["file_token"] = "https://example.com/resource"
		}},
		{"minute URL key", func(request *DeliveryRequest) {
			request.Package.Sources["minute_url"] = "https://example.com/resource"
		}},
		{"minute URL camel case key", func(request *DeliveryRequest) {
			request.Package.Sources["minuteURL"] = "https://example.com/resource"
		}},
		{"API key source", func(request *DeliveryRequest) {
			request.Package.Sources["api_key"] = "https://example.com/resource"
		}},
		{"invalid restricted digest", func(request *DeliveryRequest) {
			request.Package.Sources["feishu_drive_ref"] = "sha256:not-a-digest"
		}},
		{"unsafe source trace", func(request *DeliveryRequest) {
			request.Package.Sources["transcription"] = "/Users/private/adapter"
		}},
		{"unexpected plain source identity", func(request *DeliveryRequest) {
			request.Package.Sources["document"] = "opaque-private-identity"
		}},
		{"private note key", func(request *DeliveryRequest) {
			request.Package.Sources["private_notes"] = "https://example.com/resource"
		}},
		{"credential source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "https://example.com/episode?access_token=SECRET"
		}},
		{"signed source reference", func(request *DeliveryRequest) {
			request.Package.Sources["document"] = "https://example.com/file?X-Amz-Signature=SECRET"
		}},
		{"legacy signed source reference", func(request *DeliveryRequest) {
			request.Package.Sources["document"] = "https://example.com/file?AWSAccessKeyId=SECRET"
		}},
		{"local transcript path", func(request *DeliveryRequest) {
			request.Package.Transcript += "\nfile:///Users/private/audio.mp3"
			request.Package.TranscriptSHA256 = digestString(request.Package.Transcript)
		}},
		{"uppercase file transcript path", func(request *DeliveryRequest) {
			request.Package.Transcript += "\nFILE:///home/private/audio.mp3"
			request.Package.TranscriptSHA256 = digestString(request.Package.Transcript)
		}},
		{"Linux transcript path", func(request *DeliveryRequest) {
			request.Package.Transcript += "\n/home/private/audio.mp3"
			request.Package.TranscriptSHA256 = digestString(request.Package.Transcript)
		}},
		{"Windows notes path", func(request *DeliveryRequest) {
			request.Package.EpisodeNotes += "\nC:\\temp\\private.txt"
			request.Package.EpisodeNotesSHA256 = digestString(request.Package.EpisodeNotes)
		}},
		{"UNC show notes path", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "\\\\server\\share\\private.txt"
		}},
		{"assigned macOS path", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "path=/Users/private/audio.mp3"
		}},
		{"workspace path", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "path=/workspace/MagicPodcast/backend/data/podcast.db"
		}},
		{"prose absolute POSIX path", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "调试文件位于 /workspace/MagicPodcast/backend/data/podcast.db，请勿上传。"
		}},
		{"single slash file URI", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "FILE:/etc/passwd"
		}},
		{"forward slash Windows path", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "cache=C:/temp/private.txt"
		}},
		{"credential assignment in transcript", func(request *DeliveryRequest) {
			request.Package.Transcript += "\nfile_token=SECRET"
			request.Package.TranscriptSHA256 = digestString(request.Package.Transcript)
		}},
		{"credential JSON in notes", func(request *DeliveryRequest) {
			request.Package.EpisodeNotes += "\n{\"access_token\": \"SECRET\"}"
			request.Package.EpisodeNotesSHA256 = digestString(request.Package.EpisodeNotes)
		}},
		{"credential show notes URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[restricted](https://example.com/doc?token=SECRET)"
		}},
		{"encoded nested loopback URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[redirect](https://redirect.example/?next=http%3A%2F%2F127.0.0.1%2Fprivate)"
		}},
		{"double encoded nested credential URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[redirect](https://redirect.example/?next=https%253A%252F%252Fexample.com%252Fprivate%253Ftoken%253DSECRET)"
		}},
		{"over encoded nested private URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[redirect](https://redirect.example/?next=http%2525252525252525253A%2525252525252525252F%2525252525252525252F127.0.0.1%2525252525252525252Fprivate)"
		}},
		{"fragment nested private URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[redirect](https://redirect.example/#next=https%3A%2F%2F10.0.0.1%2Fprivate)"
		}},
		{"scheme relative nested loopback URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[redirect](https://redirect.example/?next=%2F%2F127.0.0.1%2Fprivate)"
		}},
		{"malformed show notes URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[broken](https://example.com/%zz)"
		}},
		{"uppercase loopback show notes URL", func(request *DeliveryRequest) {
			request.Package.ShowNotes = "[restricted](HTTP://127.0.0.1/private)"
		}},
		{"loopback source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "http://127.0.0.1/episode"
		}},
		{"IPv6 loopback source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "http://[::1]/episode"
		}},
		{"private source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "https://10.0.0.1/episode"
		}},
		{"decimal loopback source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "http://2130706433/episode"
		}},
		{"hex loopback source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "http://0x7f000001/episode"
		}},
		{"octal loopback source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "http://0177.0.0.1/episode"
		}},
		{"short loopback source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "http://127.1/episode"
		}},
		{"local hostname source URL", func(request *DeliveryRequest) {
			request.Package.SourceURL = "https://podcast.local/episode"
		}},
		{"Feishu source URL", func(request *DeliveryRequest) {
			request.Package.Sources["document"] = "https://example.larkoffice.com/minutes/token"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validIMAManualImportRequest()
			test.mutate(&request)
			bridge, err := NewIMAManualImportBridge(t.TempDir())
			require.NoError(t, err)
			_, err = bridge.Deliver(context.Background(), request)
			require.Error(t, err)
		})
	}
}

func TestIMAManualImportBridgeAllowsCJKProseSlashes(t *testing.T) {
	request := validIMAManualImportRequest()
	request.Package.EpisodeTitle = "支持输入/输出格式"
	request.Package.PodcastTitle = "产品/设计"
	request.Package.ShowNotes = "支持音频/视频与导入/导出。"
	request.Package.Transcript += "\n支持输入/输出格式。\n"
	request.Package.TranscriptSHA256 = digestString(request.Package.Transcript)
	request.Package.EpisodeNotes += "\n- 比较之前/之后的结果\n"
	request.Package.EpisodeNotesSHA256 = digestString(request.Package.EpisodeNotes)
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)

	_, err = bridge.Deliver(context.Background(), request)
	require.NoError(t, err)
}

func TestIMAManualImportBridgeAllowsRootRelativeShowNoteLinks(t *testing.T) {
	request := validIMAManualImportRequest()
	request.Package.ShowNotes = "继续阅读 [单集页面](/episodes/1) 和 ![封面](/images/cover.jpg)。"
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)

	_, err = bridge.Deliver(context.Background(), request)
	require.NoError(t, err)
	files := readIMAPackageFiles(
		t,
		filepath.Join(bridge.root, "packages", request.DeliveryKey),
	)
	require.Contains(t, string(files["knowledge.md"]), "[单集页面](/episodes/1)")
	require.Contains(t, string(files["knowledge.md"]), "![封面](/images/cover.jpg)")
}

func TestIMAManualImportBridgeAllowsNestedPublicURLs(t *testing.T) {
	request := validIMAManualImportRequest()
	request.Package.ShowNotes = "[redirect](https://redirect.example/?next=https%3A%2F%2Fpublic.example%2Fresource)"
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)

	_, err = bridge.Deliver(context.Background(), request)
	require.NoError(t, err)
}

func TestIMAManualImportBridgeAllowsNumericLabelsInPublicDomains(t *testing.T) {
	for _, sourceURL := range []string{
		"https://2130706433.example.com/episode",
		"https://0x7f000001.example.com/episode",
	} {
		t.Run(sourceURL, func(t *testing.T) {
			request := validIMAManualImportRequest()
			request.Package.SourceURL = sourceURL
			request.Package.Sources["episode"] = sourceURL
			bridge, err := NewIMAManualImportBridge(t.TempDir())
			require.NoError(t, err)

			_, err = bridge.Deliver(context.Background(), request)
			require.NoError(t, err)
		})
	}
}

func TestEngineKeepsManualImportDeliveryPending(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))
	episode := createProcessingEpisode(t, db, true, "ima-manual-status")
	run := startProcessingRun(t, service, episode.ID)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", run.ID).
		Updates(map[string]any{
			"status":      models.ProcessingRunStatusCompleted,
			"finished_at": now,
			"updated_at":  now,
		}).Error)
	artifact := models.EpisodeArtifactSet{
		RunID:            run.ID,
		EpisodeID:        episode.ID,
		PipelineVersion:  run.PipelineVersion,
		RootPath:         "/managed/artifacts",
		ManifestPath:     "manifest.json",
		ManifestSHA256:   strings.Repeat("1", 64),
		TranscriptSHA256: digestString("# Transcript\n"),
		NotesSHA256:      digestString("# Episode notes\n"),
		IsCurrent:        true,
		CreatedAt:        now,
	}
	require.NoError(t, db.Create(&artifact).Error)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(service, &fakeTranscriber{}, &fakeRuntime{}, store, nil)
	require.NoError(t, err)
	bridge, err := NewIMAManualImportBridge(t.TempDir())
	require.NoError(t, err)
	pkg := validIMAManualImportRequest().Package
	pkg.RunID = run.ID
	pkg.EpisodeID = episode.ID
	pkg.PipelineVersion = run.PipelineVersion
	pkg.ArtifactGeneratedAt = artifact.CreatedAt
	pkg.ManifestSHA256 = artifact.ManifestSHA256
	pkg.Transcript = "# Transcript\n"
	pkg.EpisodeNotes = "# Episode notes\n"
	pkg.TranscriptSHA256 = artifact.TranscriptSHA256
	pkg.EpisodeNotesSHA256 = artifact.NotesSHA256

	binding := BridgeBinding{Destination: "manual-import", Adapter: bridge}
	require.NoError(t, engine.deliver(context.Background(), artifact, pkg, binding))
	require.NoError(t, engine.deliver(context.Background(), artifact, pkg, binding))

	detail, err := service.GetProcessingRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Len(t, detail.Deliveries, 1)
	delivery := detail.Deliveries[0]
	require.Equal(t, models.DeliveryStatusPending, delivery.Status)
	require.Equal(t, 2, delivery.AttemptCount)
	require.Nil(t, delivery.DeliveredAt)
	require.Empty(t, delivery.PublicURL)
	require.NotContains(t, delivery.RemoteRef, bridge.root)
	require.Equal(t, models.ProcessingRunStatusCompleted, detail.Run.Status)
	entries, err := os.ReadDir(filepath.Join(bridge.root, "packages"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func validIMAManualImportRequest() DeliveryRequest {
	transcript := "# Transcript\n\n[00:00] 可核对内容\n"
	notes := "# Episode notes\n\n- 关键观点\n"
	return DeliveryRequest{
		ArtifactSetID: 42,
		DeliveryKey:   strings.Repeat("a", 64),
		Destination:   "manual-import",
		Package: KnowledgePackage{
			RunID:               7,
			EpisodeID:           9,
			EpisodeTitle:        "确定性播客加工",
			PodcastTitle:        "MagicPodcast 测试节目",
			PublishedAt:         time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC),
			SourceURL:           "https://example.com/episodes/9",
			ShowNotes:           "公开 Show Notes 与 [资料](https://example.com/home/resource)。",
			PipelineVersion:     "focus-v1",
			ArtifactGeneratedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
			ManifestSHA256:      strings.Repeat("1", 64),
			TranscriptSHA256:    digestString(transcript),
			EpisodeNotesSHA256:  digestString(notes),
			Transcript:          transcript,
			EpisodeNotes:        notes,
			Sources: map[string]string{
				"episode": "https://example.com/episodes/9",
			},
		},
	}
}

func readIMAPackageFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	for _, entry := range entries {
		content, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
		require.NoError(t, readErr)
		files[entry.Name()] = content
	}
	return files
}
