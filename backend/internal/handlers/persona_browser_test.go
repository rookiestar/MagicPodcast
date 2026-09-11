package handlers_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/config"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/database"
	"magicpodcast/internal/episodecopilot"
	"magicpodcast/internal/models"
	"magicpodcast/internal/personidentity"
	"magicpodcast/internal/processing"
	"magicpodcast/internal/router"
)

// Opt-in real browser harness: imports original transcripts, never identity
// replay, into a new isolated database. The ordinary API handles preparation.
func TestPersonaRealBrowser(t *testing.T) {
	root := os.Getenv("PERSONA_BROWSER_DIR")
	if root == "" {
		t.Skip("set a new PERSONA_BROWSER_DIR and authorized PERSONA_REAL_CORPUS")
	}
	require.NotEmpty(t, os.Getenv("PERSONA_REAL_CORPUS"))
	require.NoError(t, os.Mkdir(root, 0700), "use a new directory; never reuse or overwrite a database")
	var sources []struct {
		EpisodeID          uint   `json:"episode_id"`
		Title              string `json:"title"`
		ShowNotes          string `json:"show_notes"`
		PodcastTitle       string `json:"podcast_title"`
		PodcastAuthor      string `json:"podcast_author"`
		PodcastDescription string `json:"podcast_description"`
		PublishedDate      string `json:"published_date"`
		Timeline           struct {
			Segments []processing.TranscriptSegment `json:"segments"`
		} `json:"timeline"`
	}
	raw, err := os.ReadFile(os.Getenv("PERSONA_REAL_CORPUS"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &sources))
	_, here, _, _ := runtime.Caller(0)
	_, err = config.Load(filepath.Join(filepath.Dir(here), "..", "..", "configs", "config.example.yaml"))
	require.NoError(t, err)
	db, err := gorm.Open(sqlite.Open(filepath.Join(root, "browser.db")+"?_foreign_keys=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))
	store, err := processing.NewDiskArtifactStore(filepath.Join(root, "artifacts"))
	require.NoError(t, err)
	podcasts := map[string]uint{}
	now := time.Now().UTC()
	for index, source := range sources {
		podcastID := podcasts[source.PodcastTitle]
		if podcastID == 0 {
			pod := models.Podcast{XYZID: fmt.Sprint(source.EpisodeID), Title: source.PodcastTitle, Author: source.PodcastAuthor, Description: source.PodcastDescription, FeedURL: fmt.Sprintf("https://example.test/%d", source.EpisodeID)}
			require.NoError(t, db.Create(&pod).Error)
			podcastID = pod.ID
			podcasts[source.PodcastTitle] = pod.ID
		}
		published, err := time.Parse(time.RFC3339, source.PublishedDate)
		if err != nil {
			published, err = time.Parse("2006-01-02", source.PublishedDate[:10])
		}
		require.NoError(t, err)
		ep := models.Episode{PodcastID: podcastID, GUID: fmt.Sprint(source.EpisodeID), Title: source.Title, ShowNotes: source.ShowNotes, PublishedDate: published}
		ep.ID = source.EpisodeID
		require.NoError(t, db.Create(&ep).Error)
		var transcript strings.Builder
		transcript.WriteString("# 逐字稿\n\n")
		for _, segment := range source.Timeline.Segments {
			fmt.Fprintf(&transcript, "%s %s\n%s\n\n", segment.Speaker, formatTranscriptClock(segment.StartMS), segment.Text)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(transcript.String())))
		run := models.EpisodeProcessingRun{EpisodeID: ep.ID, ProcessingKey: digest, AudioDigest: digest, PipelineVersion: processing.NativeMinutesPipelineVersion, TriggerSource: models.ProcessingTriggerManual, Status: models.ProcessingRunStatusCompleted, CurrentStep: processing.StepArtifactPublish, AttemptCount: 1, MaxAttempts: 3, RetryDeadlineAt: now.Add(time.Hour), FinishedAt: &now, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, db.Create(&run).Error)
		artifact, err := store.Publish(context.Background(), processing.ArtifactPublishRequest{RunID: run.ID, EpisodeID: ep.ID, AudioDigest: digest, PipelineVersion: processing.NativeMinutesPipelineVersion, NativeMinutes: true, MinutesSummary: "# 隔离验收\n\n本次只导入授权的原始逐字稿，不生成纪要。", Transcript: transcript.String(), TranscriptSegments: source.Timeline.Segments, TranscriptionAdapter: "authorized-corpus-import", TranscriptionVersion: "identity-335", SkillVersions: map[string]string{"import": "identity-335"}, GeneratedAt: now})
		require.NoError(t, err)
		row := models.EpisodeArtifactSet{RunID: run.ID, EpisodeID: ep.ID, PipelineVersion: processing.NativeMinutesPipelineVersion, RootPath: artifact.RootPath, ManifestPath: artifact.ManifestPath, ManifestSHA256: artifact.ManifestSHA256, AudioSHA256: artifact.AudioSHA256, MinutesSummarySHA256: artifact.MinutesSummarySHA256, TranscriptSHA256: artifact.TranscriptSHA256, TranscriptTimelineSHA256: artifact.TranscriptTimelineSHA256, IsCurrent: true, CreatedAt: now}
		require.NoError(t, db.Create(&row).Error)
		focus := models.QueueStateFocus
		position := int64(index + 1)
		require.NoError(t, db.Create(&models.EpisodeTriageDecision{EpisodeID: ep.ID, State: models.TriageStateShortlisted, DecidedAt: now, QueueState: &focus, QueuePosition: &position, QueueUpdatedAt: &now}).Error)
		// Reproduce old persisted candidates, without pre-filling corrected names.
		if ep.ID == 66314 {
			for i, name := range []string{"战略学", "这个观点", "因为纯粹我自己", "敢用的", "觉得绝对要有希望的", "觉得", "比较乐观的", "小俊", "曾鸣"} {
				p := models.Person{StableKey: fmt.Sprintf("old-%d", i), DisplayName: name}
				require.NoError(t, db.Create(&p).Error)
				require.NoError(t, db.Create(&models.EpisodeAppearance{PersonID: p.ID, EpisodeID: ep.ID, SourceVersion: fmt.Sprintf("artifact-%d", row.ID), Role: "unknown", Status: "confirmed", EvidenceKind: "legacy-rule"}).Error)
			}
		}
	}
	require.NoError(t, db.Where(models.ConsumptionQueueOrder{QueueState: models.QueueStateFocus}).Assign(models.ConsumptionQueueOrder{Revision: 1, UpdatedAt: now}).FirstOrCreate(&models.ConsumptionQueueOrder{}).Error)
	python := filepath.Join(os.Getenv("HOME"), ".codex", "venv-openai-codex-0.147.0", "bin", "python")
	workRoot := filepath.Join(root, "runtime")
	require.NoError(t, os.MkdirAll(workRoot, 0700))
	host, err := codexruntime.NewProcessHost(codexruntime.ProcessHostConfig{Command: []string{python, filepath.Join(filepath.Dir(here), "..", "codexruntime", "runtime_host.py")}, WorkRoot: workRoot, Profiles: codexruntime.DefaultProfiles(), Environment: map[string]string{"PATH": filepath.Dir(python) + string(os.PathListSeparator) + os.Getenv("PATH")}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close(context.Background()) })
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	people, err := personidentity.NewService(db, personidentity.NewRuntimeSuggester(host, workRoot), search)
	require.NoError(t, err)
	people.WithArtifactReader(store)
	loader, err := episodecopilot.NewGORMContextLoader(db, store)
	require.NoError(t, err)
	copilot, err := episodecopilot.NewService(loader, host, workRoot, episodecopilot.WithLibrary(people, search))
	require.NoError(t, err)
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	engine := router.SetupRouter(router.WithEpisodeCopilotModule(copilot), router.WithPersonIdentityModule(people), router.WithProcessingModule(processing.NewService(db, processing.WithArtifactReader(store)), nil))
	address := os.Getenv("PERSONA_BROWSER_ADDRESS")
	if address == "" {
		address = "127.0.0.1:18145"
	}
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(engine)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	require.NoError(t, os.WriteFile(filepath.Join(root, "server.txt"), []byte(server.URL), 0600))
	t.Logf("isolated browser API %s; stop by creating %s", server.URL, filepath.Join(root, "stop"))
	for {
		if _, err := os.Stat(filepath.Join(root, "stop")); err == nil {
			return
		}
		time.Sleep(time.Second)
	}
}
