package dataprofile

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExportAndRefreshSanitizesAndDoesNotSwitchActiveProfile(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	sourcePath := filepath.Join(t.TempDir(), "production-like.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, sourcePath, 0o600))
	sourceDB, err := sql.Open("sqlite3", sourcePath)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		INSERT INTO sync_configs(id, config_key, config_value, created_at, updated_at)
		VALUES (1, 'access_token', 'TOP-SECRET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		INSERT INTO feed_user_agent_gates(
			domain, user_agent_fingerprint, state, detected_at, probe_eligible_at,
			last_probe_result, recovery_success_count, approved_by, updated_at
		) VALUES ('feeds.example', 'fingerprint', 'blocked', 1, 2, '', 0, 'owner@example.com', 3)`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		INSERT INTO feed_user_agent_gate_audits(
			domain, user_agent_fingerprint, action, mode, actor, result, created_at
		) VALUES ('feeds.example', 'fingerprint', 'approve_probe', 'apply', 'owner@example.com', 'approved', 3)`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		INSERT INTO workflows(
			id, created_at, updated_at, name, description, schedule, scope_type,
			scope_config, rules_config, is_enabled, publish_to_homepage, report_type
		) VALUES (
			9001, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'private workflow', '', '0 0 8 * * *',
			'custom_sources',
			'{"custom_urls":["https://user:secret@private.example/feed?token=TOP-SECRET"]}',
			'{"llm_user_prompt":"private thought","keywords":"AI"}',
			0, 0, ''
		)`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		INSERT INTO podcast_alternative_feeds(
			id, created_at, updated_at, podcast_id, main_feed_url, identity_key,
			alternative_feed_url, status, verification, unavailable_reason, verified_at
		) VALUES (
			9001, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1001,
			'https://private.example/main?token=TOP-SECRET', 'fixture-podcast-1001',
			'https://private.example/alternative?token=TOP-SECRET',
			'verified', 'guid_match', '', CURRENT_TIMESTAMP
		)`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		UPDATE podcasts
		SET feed_url = 'https://user:secret@private.example/feed?token=TOP-SECRET',
		    newest_enclosure_url = 'https://private.example/audio?token=TOP-SECRET',
		    cover_url = 'https://private.example/cover.jpg?signature=TOP-SECRET',
		    link = 'https://private.example/podcast?token=TOP-SECRET'
		WHERE id = 1001`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		UPDATE episodes
		SET medium_url = 'https://private.example/episode.mp3?token=TOP-SECRET',
		    show_notes = '<a href="https://user:secret@private.example/note?token=TOP-SECRET">private</a>',
		    content = 'Listen at https://private.example/content?signature=TOP-SECRET.',
		    link = 'https://private.example/episode-page?signature=TOP-SECRET',
		    image_url = 'https://private.example/episode-cover.jpg?token=TOP-SECRET'
		WHERE id = 2001`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		INSERT INTO jobs (
			id, created_at, updated_at, workflow_id, status, podcasts_processed,
			episodes_found, episodes_created, episodes_matched, error_count, triggered_by
		) VALUES (
			9001, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 9001, 'completed', 1,
			1, 0, 1, 0, 'manual'
		)`)
	require.NoError(t, err)
	_, err = sourceDB.Exec(`
		INSERT INTO reports (
			id, created_at, updated_at, job_id, title, content, summary,
			episodes_count, podcasts_count, matched_count, time_range_start,
			time_range_end, time_range_mode, generated_at, format, file_size,
			publish_to_homepage, report_type, workflow_name, structured_episodes,
			llm_summary, llm_model_used, llm_tokens_used, llm_error
		) VALUES (
			9001, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 9001, 'Private URL report',
			'[report](https://private.example/report?X-Amz-Signature=TOP-SECRET)',
			'https://user:secret@private.example/summary?token=TOP-SECRET',
			1, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'manual',
			CURRENT_TIMESTAMP, 'markdown', 1, 1, 'daily', 'Fixture workflow',
			'[{"episode_id":2001,"link":"https://private.example/episode?signature=TOP-SECRET","image_url":"https://private.example/image.jpg?token=TOP-SECRET"}]',
			'https://private.example/llm?api_key=TOP-SECRET', 'fixture', 1, ''
		)`)
	require.NoError(t, err)
	require.NoError(t, sourceDB.Close())
	sourceHashBefore, err := SHA256File(sourcePath)
	require.NoError(t, err)

	transferDir := t.TempDir()
	exported, err := ExportSanitizedSnapshot(
		sourcePath,
		transferDir,
		"snapshot-20260812T010000Z",
		"2026-08-12T01:00:00Z",
	)
	require.NoError(t, err)
	require.Equal(t, SanitizerVersion, exported.Manifest.SanitizerVersion)
	exportDB, err := sql.Open("sqlite3", "file:"+exported.DatabasePath+"?mode=ro")
	require.NoError(t, err)
	var sensitiveRows int
	require.NoError(t, exportDB.QueryRow("SELECT COUNT(*) FROM sync_configs").Scan(&sensitiveRows))
	require.Zero(t, sensitiveRows)
	var approvedBy string
	require.NoError(t, exportDB.QueryRow("SELECT approved_by FROM feed_user_agent_gates LIMIT 1").Scan(&approvedBy))
	require.Empty(t, approvedBy)
	var actor string
	require.NoError(t, exportDB.QueryRow("SELECT actor FROM feed_user_agent_gate_audits LIMIT 1").Scan(&actor))
	require.Empty(t, actor)
	var sanitizedFeed string
	require.NoError(t, exportDB.QueryRow("SELECT feed_url FROM podcasts WHERE id = 1001").Scan(&sanitizedFeed))
	require.Equal(t, "snapshot://podcast/1001", sanitizedFeed)
	var podcastCoverURL, podcastLink string
	require.NoError(t, exportDB.QueryRow(`
		SELECT cover_url, link FROM podcasts WHERE id = 1001`,
	).Scan(&podcastCoverURL, &podcastLink))
	require.Equal(t, "https://private.example/cover.jpg", podcastCoverURL)
	require.Equal(t, "https://private.example/podcast", podcastLink)
	var sanitizedMediumURL string
	require.NoError(t, exportDB.QueryRow("SELECT medium_url FROM episodes WHERE id = 2001").Scan(&sanitizedMediumURL))
	require.Empty(t, sanitizedMediumURL)
	var sanitizedShowNotes, sanitizedContent string
	require.NoError(t, exportDB.QueryRow(`
		SELECT show_notes, content FROM episodes WHERE id = 2001`,
	).Scan(&sanitizedShowNotes, &sanitizedContent))
	require.Contains(t, sanitizedShowNotes, `href="https://private.example/note"`)
	require.NotContains(t, sanitizedShowNotes, "TOP-SECRET")
	require.Contains(t, sanitizedContent, "https://private.example/content.")
	require.NotContains(t, sanitizedContent, "TOP-SECRET")
	var episodeLink, episodeImageURL string
	require.NoError(t, exportDB.QueryRow(`
		SELECT link, image_url FROM episodes WHERE id = 2001`,
	).Scan(&episodeLink, &episodeImageURL))
	require.Equal(t, "https://private.example/episode-page", episodeLink)
	require.Equal(t, "https://private.example/episode-cover.jpg", episodeImageURL)
	var reportContent, reportSummary, reportStructured, reportLLMSummary string
	require.NoError(t, exportDB.QueryRow(`
		SELECT content, summary, structured_episodes, llm_summary
		FROM reports WHERE id = 9001`,
	).Scan(&reportContent, &reportSummary, &reportStructured, &reportLLMSummary))
	require.Contains(t, reportContent, "https://private.example/report")
	require.NotContains(t, reportContent, "TOP-SECRET")
	require.Equal(t, "https://private.example/summary", reportSummary)
	require.NotContains(t, reportStructured, "TOP-SECRET")
	require.Contains(t, reportStructured, "https://private.example/episode")
	require.Contains(t, reportStructured, "https://private.example/image.jpg")
	require.Equal(t, "https://private.example/llm", reportLLMSummary)
	var customURLs, llmUserPrompt any
	require.NoError(t, exportDB.QueryRow(`
		SELECT
			json_extract(COALESCE(scope_config, '{}'), '$.custom_urls'),
			json_extract(COALESCE(rules_config, '{}'), '$.llm_user_prompt')
		FROM workflows WHERE id = 9001`).Scan(&customURLs, &llmUserPrompt))
	require.Nil(t, customURLs)
	require.Nil(t, llmUserPrompt)
	var alternativeURL string
	require.NoError(t, exportDB.QueryRow(`
		SELECT alternative_feed_url FROM podcast_alternative_feeds WHERE id = 9001`).Scan(&alternativeURL))
	require.Empty(t, alternativeURL)
	require.NoError(t, exportDB.Close())
	sourceHashAfter, err := SHA256File(sourcePath)
	require.NoError(t, err)
	require.Equal(t, sourceHashBefore, sourceHashAfter)

	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: home,
		Port:        port,
		Timeout:     30 * time.Second,
	}
	t.Cleanup(func() {
		if state, readErr := controller.readState(); readErr == nil {
			_ = controller.stop(state)
		}
	})
	active, err := controller.UseFixture(context.Background())
	require.NoError(t, err)
	refreshed, err := controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       transferDir,
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, LocalDirectoryTransfer{})
	require.NoError(t, err)
	require.Equal(t, "snapshot-20260812T010000Z", refreshed.ID)

	after, err := controller.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, active.InstanceID, after.InstanceID)
	require.Equal(t, "fixture", after.Profile)
	latest, err := ResolveSnapshot(home, "latest")
	require.NoError(t, err)
	require.Equal(t, refreshed.ID, latest.ID)
}

func TestDuplicateRefreshDoesNotMoveLatest(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	transferDir := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		transferDir,
		"snapshot-duplicate",
		"2026-08-12T02:00:00Z",
	)
	require.NoError(t, err)
	controller := Controller{ProjectDir: t.TempDir(), ProfileHome: home, Port: 18080, Timeout: time.Second}
	request := RefreshRequest{Source: transferDir, Confirmation: RefreshConfirmation, Keep: 3}
	first, err := controller.RefreshSnapshot(context.Background(), request, LocalDirectoryTransfer{})
	require.NoError(t, err)
	_, err = controller.RefreshSnapshot(context.Background(), request, LocalDirectoryTransfer{})
	require.ErrorContains(t, err, "already exists")
	latest, latestErr := ResolveSnapshot(home, "latest")
	require.NoError(t, latestErr)
	require.Equal(t, first.ID, latest.ID)
}

func TestRefreshFailuresDoNotPublishOrChangeLatest(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	first, err := publishPreparedTestSnapshot(t, home, "snapshot-existing", fixture.DatabasePath, "2026-08-10T00:00:00Z", SanitizerVersion)
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
	}
	require.NoError(t, controller.writeLatest(first))

	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source: t.TempDir(), Confirmation: "", Keep: 3,
	}, LocalDirectoryTransfer{})
	require.ErrorContains(t, err, "explicit")
	latest, latestErr := ResolveSnapshot(home, "latest")
	require.NoError(t, latestErr)
	require.Equal(t, first.ID, latest.ID)

	partial := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(partial, "magicpodcast.db"), []byte("partial"), 0o600))
	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source: partial, Confirmation: RefreshConfirmation, Keep: 3,
	}, LocalDirectoryTransfer{})
	require.Error(t, err)
	latest, latestErr = ResolveSnapshot(home, "latest")
	require.NoError(t, latestErr)
	require.Equal(t, first.ID, latest.ID)
}

func TestRefreshTransferAdapterFailureDoesNotPublishOrChangeLatest(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	first, err := publishPreparedTestSnapshot(t, home, "snapshot-existing", fixture.DatabasePath, "2026-08-10T00:00:00Z", SanitizerVersion)
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
	}
	require.NoError(t, controller.writeLatest(first))

	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       "configured-adapter",
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, CommandTransfer{Command: []string{"/usr/bin/false"}})
	require.ErrorContains(t, err, "transfer adapter failed")
	latest, latestErr := ResolveSnapshot(home, "latest")
	require.NoError(t, latestErr)
	require.Equal(t, first.ID, latest.ID)
}

func TestRefreshFailuresKeepActiveAPIAvailableAndLatestUnchanged(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	previous, err := publishPreparedTestSnapshot(
		t,
		home,
		"snapshot-existing",
		fixture.DatabasePath,
		"2026-08-10T00:00:00Z",
		SanitizerVersion,
	)
	require.NoError(t, err)
	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: home,
		Port:        port,
		Timeout:     30 * time.Second,
	}
	require.NoError(t, controller.writeLatest(previous))
	active, err := controller.UseSnapshot(context.Background(), previous.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		if state, readErr := controller.readState(); readErr == nil {
			_ = controller.stop(state)
		}
	})

	assertUnchanged := func() {
		t.Helper()
		status, statusErr := controller.Status(context.Background())
		require.NoError(t, statusErr)
		require.True(t, status.Ready)
		require.Equal(t, active.InstanceID, status.InstanceID)
		require.Equal(t, "snapshot", status.Profile)
		require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")
		latest, latestErr := ResolveSnapshot(home, "latest")
		require.NoError(t, latestErr)
		require.Equal(t, previous.ID, latest.ID)
	}

	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       "configured-adapter",
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, failingTransfer{err: errors.New("network unavailable")})
	require.ErrorContains(t, err, "network unavailable")
	assertUnchanged()

	checksumBundle := t.TempDir()
	exported, err := ExportSanitizedSnapshot(
		fixture.DatabasePath,
		checksumBundle,
		"snapshot-checksum-mismatch",
		"2026-08-12T05:00:00Z",
	)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(exported.DatabasePath, 0o600))
	file, err := os.OpenFile(exported.DatabasePath, os.O_WRONLY|os.O_APPEND, 0)
	require.NoError(t, err)
	_, err = file.Write([]byte("tamper"))
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.NoError(t, os.Chmod(exported.DatabasePath, 0o400))
	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       checksumBundle,
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, LocalDirectoryTransfer{})
	require.ErrorContains(t, err, "checksum")
	assertUnchanged()

	partialBundle := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(partialBundle, "magicpodcast.db"), []byte("partial"), 0o600))
	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       partialBundle,
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, LocalDirectoryTransfer{})
	require.Error(t, err)
	assertUnchanged()
}

type failingTransfer struct {
	err error
}

func (transfer failingTransfer) Fetch(context.Context, string) (TransferResult, error) {
	return TransferResult{}, transfer.err
}

func TestCommandTransferCleansOwnedStagingOnSuccessAndFailure(t *testing.T) {
	fixtureHome := t.TempDir()
	fixture, err := EnsureFixture(fixtureHome)
	require.NoError(t, err)
	for _, testCase := range []struct {
		name       string
		tamper     bool
		wantErr    string
		snapshotID string
	}{
		{name: "success", snapshotID: "snapshot-command-clean-success"},
		{name: "verification failure", tamper: true, wantErr: "checksum", snapshotID: "snapshot-command-clean-failure"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			home := t.TempDir()
			staging := filepath.Join(t.TempDir(), "adapter-staging")
			require.NoError(t, os.Mkdir(staging, 0o700))
			exported, exportErr := ExportSanitizedSnapshot(
				fixture.DatabasePath,
				staging,
				testCase.snapshotID,
				"2026-08-12T06:00:00Z",
			)
			require.NoError(t, exportErr)
			if testCase.tamper {
				require.NoError(t, os.Chmod(exported.DatabasePath, 0o600))
				file, openErr := os.OpenFile(exported.DatabasePath, os.O_WRONLY|os.O_APPEND, 0)
				require.NoError(t, openErr)
				_, writeErr := file.Write([]byte("tamper"))
				require.NoError(t, writeErr)
				require.NoError(t, file.Close())
				require.NoError(t, os.Chmod(exported.DatabasePath, 0o400))
			}
			adapter := filepath.Join(t.TempDir(), "adapter")
			recordPath := filepath.Join(t.TempDir(), "staging-path")
			require.NoError(t, os.WriteFile(adapter, []byte(
				"#!/bin/sh\nprintf '%s\\n' \"$4\" > \"$1\"\n"+
					"cp \"$2\" \"$4/magicpodcast.db\"\n"+
					"cp \"$3\" \"$4/manifest.json\"\n",
			), 0o700))
			adapter, err = filepath.EvalSymlinks(adapter)
			require.NoError(t, err)
			controller := Controller{
				ProjectDir:  t.TempDir(),
				ProfileHome: home,
				Port:        18080,
				Timeout:     time.Second,
			}
			_, refreshErr := controller.RefreshSnapshot(context.Background(), RefreshRequest{
				Source:       "configured-adapter",
				Confirmation: RefreshConfirmation,
				Keep:         3,
			}, CommandTransfer{Command: []string{
				adapter,
				recordPath,
				exported.DatabasePath,
				exported.ManifestPath,
			}})
			if testCase.wantErr == "" {
				require.NoError(t, refreshErr)
			} else {
				require.ErrorContains(t, refreshErr, testCase.wantErr)
			}
			recorded, readErr := os.ReadFile(recordPath)
			require.NoError(t, readErr)
			_, statErr := os.Stat(strings.TrimSpace(string(recorded)))
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestRefreshLatestCommitFailureRemovesUnpublishedSnapshot(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	first, err := publishPreparedTestSnapshot(t, home, "snapshot-existing", fixture.DatabasePath, "2026-08-10T00:00:00Z", SanitizerVersion)
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
	}
	require.NoError(t, controller.writeLatest(first))
	transferDir := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		transferDir,
		"snapshot-unpublished",
		"2026-08-12T04:00:00Z",
	)
	require.NoError(t, err)
	controller.writeLatestHook = func(Snapshot) error {
		return errors.New("injected latest commit failure")
	}

	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       transferDir,
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, LocalDirectoryTransfer{})
	require.ErrorContains(t, err, "injected latest commit failure")

	latest, latestErr := ResolveSnapshot(home, "latest")
	require.NoError(t, latestErr)
	require.Equal(t, first.ID, latest.ID)
	_, statErr := os.Stat(filepath.Join(home, "snapshots", "snapshot-unpublished"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestSanitizerFailsClosedForUnknownSensitiveColumn(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "candidate.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, path, 0o600))
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = db.Exec("ALTER TABLE podcasts ADD COLUMN private_token TEXT")
	require.NoError(t, err)

	err = SanitizeSnapshot(db)
	require.ErrorContains(t, err, "podcasts.private_token")
	require.NoError(t, db.Close())
}

func TestSanitizerFailsClosedForUnknownColumnsRegardlessOfName(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	for _, column := range []string{"private_payload", "owner_id", "harmless_new_field"} {
		t.Run(column, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "candidate.db")
			require.NoError(t, copyRegularFile(fixture.DatabasePath, path, 0o600))
			db, openErr := sql.Open("sqlite3", path)
			require.NoError(t, openErr)
			_, alterErr := db.Exec("ALTER TABLE podcasts ADD COLUMN " + column + " TEXT")
			require.NoError(t, alterErr)

			sanitizeErr := SanitizeSnapshot(db)
			require.ErrorContains(t, sanitizeErr, "podcasts."+column)
			require.NoError(t, db.Close())
		})
	}
}

func TestSanitizerFailsClosedForMissingReviewedColumn(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "candidate.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, path, 0o600))
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = db.Exec("ALTER TABLE sync_configs RENAME TO sync_configs_old")
	require.NoError(t, err)
	_, err = db.Exec(`
		CREATE TABLE sync_configs (
			id INTEGER PRIMARY KEY,
			config_key TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`)
	require.NoError(t, err)

	sanitizeErr := SanitizeSnapshot(db)
	require.ErrorContains(t, sanitizeErr, "missing:sync_configs.config_value")
	require.NoError(t, db.Close())
}

type cleanupFailingTransfer struct {
	directory string
}

func (transfer cleanupFailingTransfer) Fetch(context.Context, string) (TransferResult, error) {
	return TransferResult{
		Directory: transfer.directory,
		Cleanup: func() error {
			return errors.New("injected transfer cleanup failure")
		},
	}, nil
}

func TestRefreshCleanupFailureDoesNotMoveLatestOrPublishSnapshot(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	previous, err := publishPreparedTestSnapshot(
		t,
		home,
		"snapshot-before-cleanup-failure",
		fixture.DatabasePath,
		"2026-08-10T00:00:00Z",
		SanitizerVersion,
	)
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
	}
	require.NoError(t, controller.writeLatest(previous))
	transferDir := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		transferDir,
		"snapshot-cleanup-failed",
		"2026-08-13T02:00:00Z",
	)
	require.NoError(t, err)

	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       "configured-adapter",
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, cleanupFailingTransfer{directory: transferDir})
	require.ErrorContains(t, err, "injected transfer cleanup failure")
	latest, latestErr := ResolveSnapshot(home, "latest")
	require.NoError(t, latestErr)
	require.Equal(t, previous.ID, latest.ID)
	_, statErr := os.Stat(filepath.Join(home, "snapshots", "snapshot-cleanup-failed"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestSanitizerSchemaFingerprintMatchesReviewedCurrentSchema(t *testing.T) {
	schema, err := currentSanitizerSchema()
	require.NoError(t, err)
	require.Equal(t, sanitizerSchemaFingerprint, schemaFingerprint(schema))
}

func TestRefreshRetentionProtectsActiveAndLatestSnapshots(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	var snapshots []Snapshot
	for index, date := range []string{"2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z", "2026-08-03T00:00:00Z"} {
		snapshot, publishErr := publishPreparedTestSnapshot(t,
			home,
			"snapshot-00"+string(rune('1'+index)),
			fixture.DatabasePath,
			date,
			SanitizerVersion,
		)
		require.NoError(t, publishErr)
		snapshots = append(snapshots, snapshot)
	}
	controller := Controller{ProfileHome: home}
	active := RuntimeState{Profile: "snapshot", SnapshotID: snapshots[0].ID}
	require.NoError(t, controller.enforceSnapshotRetention(1, snapshots[2].ID, "", active))

	_, err = ResolveSnapshot(home, snapshots[0].ID)
	require.NoError(t, err)
	_, err = ResolveSnapshot(home, snapshots[2].ID)
	require.NoError(t, err)
	_, err = ResolveSnapshot(home, snapshots[1].ID)
	require.Error(t, err)
}

func TestRetentionCanRollbackAllQuarantinedSnapshots(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	var snapshots []Snapshot
	for index, date := range []string{
		"2026-08-01T00:00:00Z",
		"2026-08-02T00:00:00Z",
		"2026-08-03T00:00:00Z",
		"2026-08-04T00:00:00Z",
	} {
		snapshot, publishErr := publishPreparedTestSnapshot(t,
			home,
			"snapshot-rollback-00"+string(rune('1'+index)),
			fixture.DatabasePath,
			date,
			SanitizerVersion,
		)
		require.NoError(t, publishErr)
		snapshots = append(snapshots, snapshot)
	}
	controller := Controller{ProfileHome: home}
	retention, err := controller.stageSnapshotRetention(1, snapshots[3].ID, "", RuntimeState{})
	require.NoError(t, err)
	require.Len(t, retention.moves, 3)
	for _, move := range retention.moves {
		_, statErr := os.Stat(move.original)
		require.ErrorIs(t, statErr, os.ErrNotExist)
	}

	require.NoError(t, retention.Rollback())
	for _, snapshot := range snapshots {
		_, resolveErr := ResolveSnapshot(home, snapshot.ID)
		require.NoError(t, resolveErr)
	}
}

func TestRefreshRetentionCommitFailureRestoresSnapshotsAndPreviousLatest(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	var existing []Snapshot
	for index, date := range []string{
		"2026-08-01T00:00:00Z",
		"2026-08-02T00:00:00Z",
		"2026-08-03T00:00:00Z",
	} {
		snapshot, publishErr := publishPreparedTestSnapshot(t,
			home,
			"snapshot-existing-00"+string(rune('1'+index)),
			fixture.DatabasePath,
			date,
			SanitizerVersion,
		)
		require.NoError(t, publishErr)
		existing = append(existing, snapshot)
	}
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
		retentionHook: func(*snapshotRetention) error {
			return errors.New("injected retention cleanup failure")
		},
	}
	require.NoError(t, controller.writeLatest(existing[2]))
	transferDir := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		transferDir,
		"snapshot-newest",
		"2026-08-13T00:00:00Z",
	)
	require.NoError(t, err)

	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       transferDir,
		Confirmation: RefreshConfirmation,
		Keep:         1,
	}, LocalDirectoryTransfer{})
	require.ErrorContains(t, err, "injected retention cleanup failure")

	latest, err := ResolveSnapshot(home, "latest")
	require.NoError(t, err)
	require.Equal(t, existing[2].ID, latest.ID)
	for _, snapshot := range existing {
		_, resolveErr := ResolveSnapshot(home, snapshot.ID)
		require.NoError(t, resolveErr)
	}
}

func TestRefreshCleanupFailureKeepsCommittedSnapshotAndLatest(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	var existing []Snapshot
	for index, date := range []string{
		"2026-08-01T00:00:00Z",
		"2026-08-02T00:00:00Z",
		"2026-08-03T00:00:00Z",
	} {
		snapshot, publishErr := publishPreparedTestSnapshot(t,
			home,
			"snapshot-cleanup-00"+string(rune('1'+index)),
			fixture.DatabasePath,
			date,
			SanitizerVersion,
		)
		require.NoError(t, publishErr)
		existing = append(existing, snapshot)
	}
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
	}
	require.NoError(t, controller.writeLatest(existing[2]))
	transferDir := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		transferDir,
		"snapshot-cleanup-newest",
		"2026-08-13T00:00:00Z",
	)
	require.NoError(t, err)

	originalRemoveAll := func(path string) error {
		return errors.New("injected physical cleanup failure")
	}
	controller.retentionHook = func(retention *snapshotRetention) error {
		retention.removeAll = originalRemoveAll
		return retention.Commit()
	}

	refreshed, err := controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       transferDir,
		Confirmation: RefreshConfirmation,
		Keep:         1,
	}, LocalDirectoryTransfer{})
	require.NoError(t, err)
	require.Equal(t, "snapshot-cleanup-newest", refreshed.ID)
	latest, err := ResolveSnapshot(home, "latest")
	require.NoError(t, err)
	require.Equal(t, refreshed.ID, latest.ID)
	garbage, err := filepath.Glob(filepath.Join(home, "snapshots", ".retention-garbage-*"))
	require.NoError(t, err)
	require.Len(t, garbage, 1, "cleanup failure should leave one hidden owned retry directory")
}

func TestRefreshActiveStateTamperRestoresPreviousLatestAndSnapshots(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	previous, err := publishPreparedTestSnapshot(t,
		home,
		"snapshot-before-state-tamper",
		fixture.DatabasePath,
		"2026-08-10T00:00:00Z",
		SanitizerVersion,
	)
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
	}
	require.NoError(t, controller.writeLatest(previous))
	transferDir := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		transferDir,
		"snapshot-after-state-tamper",
		"2026-08-13T00:00:00Z",
	)
	require.NoError(t, err)
	controller.afterPublishHook = func() {
		require.NoError(t, os.MkdirAll(filepath.Dir(controller.statePath()), 0o700))
		require.NoError(t, os.WriteFile(controller.statePath(), []byte("{}"), 0o600))
	}

	_, err = controller.RefreshSnapshot(context.Background(), RefreshRequest{
		Source:       transferDir,
		Confirmation: RefreshConfirmation,
		Keep:         3,
	}, LocalDirectoryTransfer{})
	require.ErrorContains(t, err, "unexpectedly activated a profile")
	latest, latestErr := ResolveSnapshot(home, "latest")
	require.NoError(t, latestErr)
	require.Equal(t, previous.ID, latest.ID)
	_, statErr := os.Stat(filepath.Join(home, "snapshots", "snapshot-after-state-tamper"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestTransferBundleRejectsSymlinkedFiles(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	realBundle := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		realBundle,
		"snapshot-transfer-symlink",
		"2026-08-13T00:00:00Z",
	)
	require.NoError(t, err)
	symlinkBundle := t.TempDir()
	require.NoError(t, os.Symlink(
		filepath.Join(realBundle, "magicpodcast.db"),
		filepath.Join(symlinkBundle, "magicpodcast.db"),
	))
	require.NoError(t, os.Symlink(
		filepath.Join(realBundle, "manifest.json"),
		filepath.Join(symlinkBundle, "manifest.json"),
	))

	_, _, err = verifyTransferBundle(symlinkBundle)
	require.ErrorContains(t, err, "must be a regular file")
}

func TestExportRequiresRealEmptyPrivateDirectory(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)

	missing := filepath.Join(t.TempDir(), "missing")
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		missing,
		"snapshot-missing-output",
		"2026-08-13T00:00:00Z",
	)
	require.ErrorContains(t, err, "inspect directory")

	nonempty := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(nonempty, "keep"), []byte("keep"), 0o600))
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		nonempty,
		"snapshot-nonempty-output",
		"2026-08-13T00:00:00Z",
	)
	require.ErrorContains(t, err, "must be empty")

	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "staging-link")
	require.NoError(t, os.Symlink(target, link))
	_, err = ExportSanitizedSnapshot(
		fixture.DatabasePath,
		link,
		"snapshot-symlink-output",
		"2026-08-13T00:00:00Z",
	)
	require.ErrorContains(t, err, "not a symlink")
}

func TestConsistentSQLiteBackupCancellationAndBusyFailureLeaveNoPublishedBundle(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)

	cancelledOutput := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ExportSanitizedSnapshotContext(
		ctx,
		fixture.DatabasePath,
		cancelledOutput,
		"snapshot-cancelled-backup",
		"2026-08-13T01:00:00Z",
	)
	require.ErrorContains(t, err, "canceled")
	assertNoPublishedTransferFiles(t, cancelledOutput)

	lockedSource := filepath.Join(t.TempDir(), "locked.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, lockedSource, 0o600))
	lockDB, err := sql.Open("sqlite3", lockedSource)
	require.NoError(t, err)
	lockConnection, err := lockDB.Conn(context.Background())
	require.NoError(t, err)
	_, err = lockConnection.ExecContext(context.Background(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	_, err = lockConnection.ExecContext(
		context.Background(),
		"UPDATE podcasts SET title = title WHERE id = 1001",
	)
	require.NoError(t, err)

	destination := filepath.Join(t.TempDir(), "busy-copy.db")
	err = consistentSQLiteBackupContext(
		context.Background(),
		lockedSource,
		destination,
		50*time.Millisecond,
	)
	require.Error(t, err)
	_, rollbackErr := lockConnection.ExecContext(context.Background(), "ROLLBACK")
	require.NoError(t, rollbackErr)
	require.NoError(t, lockConnection.Close())
	require.NoError(t, lockDB.Close())
	_ = os.Remove(destination)
}

func assertNoPublishedTransferFiles(t *testing.T, directory string) {
	t.Helper()
	_, databaseErr := os.Stat(filepath.Join(directory, "magicpodcast.db"))
	require.ErrorIs(t, databaseErr, os.ErrNotExist)
	_, manifestErr := os.Stat(filepath.Join(directory, "manifest.json"))
	require.ErrorIs(t, manifestErr, os.ErrNotExist)
}

func TestLocalTransferRejectsSymlinkedDirectory(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "transfer-link")
	require.NoError(t, os.Symlink(target, link))

	_, err := (LocalDirectoryTransfer{}).Fetch(context.Background(), link)
	require.ErrorContains(t, err, "not a symlink")
}

func TestCommandTransferRequiresAbsoluteRegularExecutable(t *testing.T) {
	_, err := (CommandTransfer{Command: []string{"relative-adapter"}}).Fetch(context.Background(), "")
	require.ErrorContains(t, err, "must be absolute")

	target := filepath.Join(t.TempDir(), "adapter")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700))
	link := filepath.Join(t.TempDir(), "adapter-link")
	require.NoError(t, os.Symlink(target, link))
	_, err = (CommandTransfer{Command: []string{link}}).Fetch(context.Background(), "")
	require.ErrorContains(t, err, "regular file")

	nonExecutable := filepath.Join(t.TempDir(), "adapter")
	require.NoError(t, os.WriteFile(nonExecutable, []byte("adapter"), 0o600))
	_, err = (CommandTransfer{Command: []string{nonExecutable}}).Fetch(context.Background(), "")
	require.ErrorContains(t, err, "not executable")
}

func TestCommandTransferDoesNotExposeAdapterOutputAndCleansFailedStaging(t *testing.T) {
	adapter := filepath.Join(t.TempDir(), "adapter")
	recordPath := filepath.Join(t.TempDir(), "staging-path")
	require.NoError(t, os.WriteFile(adapter, []byte(
		"#!/bin/sh\nprintf '%s\\n' \"$2\" > \"$1\"\nprintf 'TOP-SECRET\\n'\nprintf 'TOP-SECRET\\n' >&2\nexit 1\n",
	), 0o700))
	adapter, err := filepath.EvalSymlinks(adapter)
	require.NoError(t, err)

	_, err = (CommandTransfer{Command: []string{adapter, recordPath}}).Fetch(context.Background(), "")
	require.ErrorContains(t, err, "transfer adapter failed")
	require.NotContains(t, err.Error(), "TOP-SECRET")
	recorded, readErr := os.ReadFile(recordPath)
	require.NoError(t, readErr)
	_, statErr := os.Stat(strings.TrimSpace(string(recorded)))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
