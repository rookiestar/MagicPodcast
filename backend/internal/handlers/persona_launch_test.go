package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/config"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/database"
	"magicpodcast/internal/episodecopilot"
	"magicpodcast/internal/models"
	"magicpodcast/internal/personidentity"
	"magicpodcast/internal/processing"
	"magicpodcast/internal/router"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPersonaIsolatedLaunchAskTwice(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	configPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "configs", "config.example.yaml")
	_, err := config.Load(configPath)
	require.NoError(t, err)
	db := openLaunchDB(t)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	people, err := personidentity.NewService(db, nil, search)
	require.NoError(t, err)
	seeded, err := personidentity.SeedBaseline(context.Background(), db, people)
	require.NoError(t, err)
	require.NoError(t, indexSeededLibrary(t, db, search, people, seeded))

	episodeID := seeded.EpisodeIDs["ep-tech-overtime"]
	zhangID := personByName(t, people, episodeID, "张三")
	store, artifactID := seedPublishedTranscript(t, db, people, episodeID)
	processingService := processing.NewService(db, processing.WithArtifactReader(store))
	loader := &staticCopilotLoader{
		scope: episodecopilot.ContextScope{
			EpisodeID:           episodeID,
			ShowNotesAvailable:  true,
			TranscriptAvailable: true,
		},
		context: episodecopilot.EpisodeContext{
			EpisodeID:    episodeID,
			EpisodeTitle: "工程师该不该把加班当文化",
			PodcastTitle: "技术漫谈",
			ShowNotes:    "主播张三和嘉宾李明讨论加班。",
			Transcript:   "我不赞成无限制加班，长期靠加班堆产出会把团队拖垮。",
		},
	}
	runtime := newLaunchRuntime(
		launchExecution{deltas: []string{"我不赞成无限制加班 [库内 S1]。"}},
		launchExecution{deltas: []string{"我不赞成无限制加班 [库内 S1]。"}},
	)
	copilot, err := episodecopilot.NewService(
		loader,
		runtime,
		t.TempDir(),
		episodecopilot.WithLibrary(people, search),
	)
	require.NoError(t, err)

	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	engine := router.SetupRouter(
		router.WithEpisodeCopilotModule(copilot),
		router.WithPersonIdentityModule(people),
		router.WithProcessingModule(processingService, nil),
	)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	if os.Getenv("PERSONA_HOLD") == "1" {
		server.Close()
		listener, listenErr := net.Listen("tcp", "127.0.0.1:18124")
		require.NoError(t, listenErr)
		held := httptest.NewUnstartedServer(engine)
		held.Listener = listener
		held.Start()
		t.Cleanup(held.Close)
		server = held
	}

	peopleBody := launchGET(t, server.URL+fmt.Sprintf("/api/v1/episodes/%d/people", episodeID))
	require.Contains(t, peopleBody, "张三")
	require.NotContains(t, peopleBody, "王总")
	transcriptBody := launchGET(t, server.URL+fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", artifactID))
	require.Contains(t, transcriptBody, "我不赞成无限制加班")
	require.Contains(t, transcriptBody, `"order":3`)
	runsBody := launchGET(t, server.URL+fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episodeID))
	require.Contains(t, runsBody, `"status":"completed"`)

	askOnce := func(label string) string {
		body := fmt.Sprintf(
			`{"question":"他对加班怎么看？","target_person_id":%d,"profile_id":"balanced"}`,
			zhangID,
		)
		return launchPOSTSSE(
			t,
			server.URL+fmt.Sprintf("/api/v1/episodes/%d/copilot/questions", episodeID),
			body,
			label,
		)
	}
	first := askOnce("launch-1")
	second := askOnce("launch-2")
	for _, payload := range []string{first, second} {
		require.Contains(t, payload, episodecopilotDisclaimer())
		require.Contains(t, payload, "库内 S")
		require.NotContains(t, payload, "待确认")
	}
	if os.Getenv("PERSONA_HOLD") == "1" {
		now := time.Now().UTC()
		focus := models.QueueStateFocus
		position := int64(1)
		require.NoError(t, db.Create(&models.EpisodeTriageDecision{
			EpisodeID:      episodeID,
			State:          models.TriageStateShortlisted,
			DecidedAt:      now,
			QueueState:     &focus,
			QueuePosition:  &position,
			QueueUpdatedAt: &now,
		}).Error)
		var seededEpisode models.Episode
		require.NoError(t, db.First(&seededEpisode, episodeID).Error)
		pendingEpisode := models.Episode{
			PodcastID:     seededEpisode.PodcastID,
			Title:         "索引尚未准备的单集",
			GUID:          "ep-index-pending",
			PublishedDate: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
		}
		require.NoError(t, db.Create(&pendingEpisode).Error)
		pendingPosition := int64(2)
		require.NoError(t, db.Create(&models.EpisodeTriageDecision{
			EpisodeID:      pendingEpisode.ID,
			State:          models.TriageStateShortlisted,
			DecidedAt:      now,
			QueueState:     &focus,
			QueuePosition:  &pendingPosition,
			QueueUpdatedAt: &now,
		}).Error)
		require.NoError(t, db.Where(models.ConsumptionQueueOrder{QueueState: focus}).
			Assign(models.ConsumptionQueueOrder{Revision: 1, UpdatedAt: now}).
			FirstOrCreate(&models.ConsumptionQueueOrder{}).Error)
		t.Logf(
			"PERSONA_SERVER %s episode=%d person=%d pending=%d artifact=%d",
			server.URL, episodeID, zhangID, pendingEpisode.ID, artifactID,
		)
		if path := os.Getenv("PERSONA_LAUNCH_LOG_DIR"); path != "" {
			_ = os.WriteFile(
				path+"/persona-server.txt",
				[]byte(fmt.Sprintf(
					"%s\n%d\n%d\n%d\n%d\n",
					server.URL, episodeID, zhangID, pendingEpisode.ID, artifactID,
				)),
				0o600,
			)
		}
		select {}
	}
}

func TestPersonaRuntimeE2E(t *testing.T) {
	if os.Getenv("PERSONA_RUNTIME_E2E") != "1" {
		t.Skip("set PERSONA_RUNTIME_E2E=1 to drive the live Codex Runtime")
	}
	python := strings.TrimSpace(os.Getenv("PERSONA_RUNTIME_PYTHON"))
	if python == "" {
		python = filepath.Join(os.Getenv("HOME"), ".codex", "venv-openai-codex-0.147.0", "bin", "python")
	}
	require.FileExists(t, python)
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	configPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "configs", "config.example.yaml")
	_, err := config.Load(configPath)
	require.NoError(t, err)
	hostScript := filepath.Join(filepath.Dir(sourceFile), "..", "codexruntime", "runtime_host.py")
	require.FileExists(t, hostScript)
	workRoot := t.TempDir()
	require.NoError(t, os.Chmod(workRoot, 0o700))

	db := openLaunchDB(t)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	people, err := personidentity.NewService(db, nil, search)
	require.NoError(t, err)
	seeded, err := personidentity.SeedBaseline(context.Background(), db, people)
	require.NoError(t, err)
	require.NoError(t, indexSeededLibrary(t, db, search, people, seeded))

	host, err := codexruntime.NewProcessHost(codexruntime.ProcessHostConfig{
		Command:  []string{python, hostScript},
		WorkRoot: workRoot,
		Profiles: codexruntime.DefaultProfiles(),
		Environment: map[string]string{
			"PATH": filepath.Dir(python) + string(os.PathListSeparator) + os.Getenv("PATH"),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close(context.Background()) })

	copilot, err := episodecopilot.NewService(
		&baselineCopilotLoader{db: db, people: people},
		host,
		workRoot,
		episodecopilot.WithLibrary(people, search),
	)
	require.NoError(t, err)
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	server := httptest.NewServer(router.SetupRouter(
		router.WithEpisodeCopilotModule(copilot),
		router.WithPersonIdentityModule(people),
	))
	t.Cleanup(server.Close)

	overtimeID := seeded.EpisodeIDs["ep-tech-overtime"]
	noTopicID := seeded.EpisodeIDs["ep-no-topic"]
	zhangID := personByName(t, people, overtimeID, "张三")
	zhaoID := personByName(t, people, noTopicID, "赵六")
	cases := []struct {
		label    string
		episode  uint
		person   uint
		question string
		want     []string
		forbid   []string
	}{
		{
			label:    "runtime-baseline",
			episode:  overtimeID,
			person:   zhangID,
			question: "他对加班怎么看？",
			want:     []string{episodecopilotDisclaimer(), "库内 S", "未额外搜索网页"},
			forbid:   []string{"待确认"},
		},
		{
			label:    "runtime-no-answer",
			episode:  noTopicID,
			person:   zhaoID,
			question: "赵六对量子计算的商业化怎么看？",
			want:     []string{episodecopilotDisclaimer(), "无法判断"},
			forbid:   []string{"待确认"},
		},
		{
			label:    "runtime-library-sufficient",
			episode:  overtimeID,
			person:   zhangID,
			question: "张三怎么看待用加班换产出？",
			want:     []string{episodecopilotDisclaimer(), "库内 S", "未额外搜索网页"},
			forbid:   []string{"待确认"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"question":%q,"target_person_id":%d,"profile_id":"balanced"}`,
				tc.question,
				tc.person,
			)
			payload := launchPOSTSSETimeout(
				t,
				server.URL+fmt.Sprintf("/api/v1/episodes/%d/copilot/questions", tc.episode),
				body,
				tc.label,
				8*time.Minute,
			)
			for _, needle := range tc.want {
				require.Contains(t, payload, needle)
			}
			for _, needle := range tc.forbid {
				require.NotContains(t, payload, needle)
			}
		})
	}
}

func episodecopilotDisclaimer() string {
	return "基于公开表达的 AI 模拟，非本人回复"
}

func openLaunchDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:persona_launch_%d?mode=memory&cache=shared&_foreign_keys=on", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))
	return db
}

func indexSeededLibrary(
	t *testing.T,
	db *gorm.DB,
	search *contentsearch.Service,
	people *personidentity.Service,
	seeded personidentity.SeededLibrary,
) error {
	t.Helper()
	for _, episodeID := range seeded.EpisodeIDs {
		listed, err := people.ListEpisodePeople(context.Background(), episodeID)
		if err != nil {
			return err
		}
		fragments := make([]contentsearch.FragmentInput, 0, len(listed.Attributions))
		for _, attribution := range listed.Attributions {
			fragments = append(fragments, contentsearch.FragmentInput{
				Order:             attribution.FragmentOrder,
				StartMS:           attribution.StartMS,
				Text:              attribution.Text,
				PersonID:          attribution.PersonID,
				AttributionStatus: attribution.Status,
			})
		}
		var episode struct {
			ShowNotes     string
			PublishedDate time.Time
		}
		if err := db.Raw("SELECT show_notes, published_date FROM episodes WHERE id = ?", episodeID).Scan(&episode).Error; err != nil {
			return err
		}
		if err := search.ReplaceEpisode(context.Background(), contentsearch.EpisodeDocument{
			EpisodeID:     episodeID,
			PublishedAt:   episode.PublishedDate,
			ShowNotes:     episode.ShowNotes,
			SourceKind:    contentsearch.SourceTranscript,
			SourceVersion: listed.SourceVersion,
			Fragments:     fragments,
			Complete:      true,
		}); err != nil {
			return err
		}
	}
	return nil
}

func seedPublishedTranscript(
	t *testing.T,
	db *gorm.DB,
	people *personidentity.Service,
	episodeID uint,
) (*processing.DiskArtifactStore, uint) {
	t.Helper()
	listed, err := people.ListEpisodePeople(context.Background(), episodeID)
	require.NoError(t, err)
	sort.Slice(listed.Attributions, func(i, j int) bool {
		return listed.Attributions[i].FragmentOrder < listed.Attributions[j].FragmentOrder
	})
	segments := make([]processing.TranscriptSegment, 0, len(listed.Attributions))
	var transcript strings.Builder
	transcript.WriteString("# 逐字稿\n\n")
	for _, attribution := range listed.Attributions {
		speaker := strings.TrimSpace(attribution.SpeakerLabel)
		if speaker == "" {
			speaker = "说话人"
		}
		segments = append(segments, processing.TranscriptSegment{
			Order:   attribution.FragmentOrder,
			Speaker: speaker,
			StartMS: attribution.StartMS,
			Text:    attribution.Text,
		})
		fmt.Fprintf(
			&transcript,
			"%s %s\n%s\n\n",
			speaker,
			formatTranscriptClock(attribution.StartMS),
			attribution.Text,
		)
	}
	store, err := processing.NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	now := time.Now().UTC()
	digest := strings.Repeat("a", 64)
	run := models.EpisodeProcessingRun{
		EpisodeID:       episodeID,
		ProcessingKey:   strings.Repeat("b", 64),
		AudioDigest:     digest,
		PipelineVersion: processing.NativeMinutesPipelineVersion,
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusCompleted,
		CurrentStep:     processing.StepArtifactPublish,
		AttemptCount:    1,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(24 * time.Hour),
		FinishedAt:      &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&run).Error)
	published, err := store.Publish(context.Background(), processing.ArtifactPublishRequest{
		RunID:                run.ID,
		EpisodeID:            episodeID,
		AudioDigest:          digest,
		PipelineVersion:      processing.NativeMinutesPipelineVersion,
		NativeMinutes:        true,
		MinutesSummary:       "# 纪要\n\n张三反对把加班当成文化。\n",
		Transcript:           transcript.String(),
		TranscriptSegments:   segments,
		TranscriptionAdapter: "fake-minutes",
		TranscriptionVersion: "fake-minutes-v1",
		SkillVersions:        map[string]string{"minutes": "skill-v1"},
		Sources:              map[string]string{"episode": "https://example.test/ep-tech-overtime"},
		RawArtifacts:         map[string][]byte{"minutes-detail.json": []byte(`{"ok":true}`)},
		GeneratedAt:          now,
	})
	require.NoError(t, err)
	artifact := models.EpisodeArtifactSet{
		RunID:                    run.ID,
		EpisodeID:                episodeID,
		PipelineVersion:          processing.NativeMinutesPipelineVersion,
		RootPath:                 published.RootPath,
		ManifestPath:             published.ManifestPath,
		ManifestSHA256:           published.ManifestSHA256,
		AudioSHA256:              published.AudioSHA256,
		MinutesSummarySHA256:     published.MinutesSummarySHA256,
		TranscriptSHA256:         published.TranscriptSHA256,
		TranscriptTimelineSHA256: published.TranscriptTimelineSHA256,
		IsCurrent:                true,
		CreatedAt:                now,
	}
	require.NoError(t, db.Create(&artifact).Error)
	people.WithArtifactReader(store)
	_, err = people.PrepareCurrent(context.Background(), episodeID)
	require.NoError(t, err)
	return store, artifact.ID
}

func formatTranscriptClock(ms int64) string {
	total := ms / 1000
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, ms%1000)
}

func personByName(t *testing.T, people *personidentity.Service, episodeID uint, name string) uint {
	t.Helper()
	listed, err := people.ListEpisodePeople(context.Background(), episodeID)
	require.NoError(t, err)
	for _, person := range listed.People {
		if person.DisplayName == name {
			return person.ID
		}
	}
	t.Fatalf("person %s not found", name)
	return 0
}

func launchGET(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Get(url)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	return string(body)
}

func launchPOSTSSE(t *testing.T, url, body, label string) string {
	t.Helper()
	return launchPOSTSSETimeout(t, url, body, label, 2*time.Minute)
}

func launchPOSTSSETimeout(t *testing.T, url, body, label string, timeout time.Duration) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	if path := os.Getenv("PERSONA_LAUNCH_LOG_DIR"); path != "" {
		_ = os.WriteFile(path+"/"+label+".log", payload, 0o600)
	}
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	return string(payload)
}

type baselineCopilotLoader struct {
	db     *gorm.DB
	people *personidentity.Service
}

func (l *baselineCopilotLoader) Describe(
	ctx context.Context,
	episodeID uint,
) (episodecopilot.ContextScope, error) {
	var episode models.Episode
	if err := l.db.WithContext(ctx).First(&episode, episodeID).Error; err != nil {
		return episodecopilot.ContextScope{}, err
	}
	listed, err := l.people.ListEpisodePeople(ctx, episodeID)
	if err != nil {
		return episodecopilot.ContextScope{}, err
	}
	return episodecopilot.ContextScope{
		EpisodeID:           episodeID,
		ShowNotesAvailable:  strings.TrimSpace(episode.ShowNotes) != "",
		TranscriptAvailable: attributionTranscript(listed) != "",
	}, nil
}

func (l *baselineCopilotLoader) Load(
	ctx context.Context,
	episodeID uint,
	_ bool,
) (episodecopilot.EpisodeContext, error) {
	var episode models.Episode
	if err := l.db.WithContext(ctx).Preload("Podcast").First(&episode, episodeID).Error; err != nil {
		return episodecopilot.EpisodeContext{}, err
	}
	listed, err := l.people.ListEpisodePeople(ctx, episodeID)
	if err != nil {
		return episodecopilot.EpisodeContext{}, err
	}
	return episodecopilot.EpisodeContext{
		EpisodeID:    episodeID,
		EpisodeTitle: episode.Title,
		PodcastTitle: episode.Podcast.Title,
		ShowNotes:    episode.ShowNotes,
		Transcript:   attributionTranscript(listed),
		PrivateNotes: episode.Notes,
	}, nil
}

func attributionTranscript(listed personidentity.EpisodePeople) string {
	rows := append([]personidentity.AttributionView(nil), listed.Attributions...)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].FragmentOrder < rows[j].FragmentOrder
	})
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Text) == "" {
			continue
		}
		speaker := strings.TrimSpace(row.SpeakerLabel)
		if speaker == "" {
			speaker = "说话人"
		}
		parts = append(parts, speaker+"："+row.Text)
	}
	return strings.Join(parts, "\n")
}

type staticCopilotLoader struct {
	scope   episodecopilot.ContextScope
	context episodecopilot.EpisodeContext
}

func (s *staticCopilotLoader) Describe(_ context.Context, episodeID uint) (episodecopilot.ContextScope, error) {
	if episodeID != s.scope.EpisodeID {
		return episodecopilot.ContextScope{EpisodeID: episodeID}, nil
	}
	return s.scope, nil
}

func (s *staticCopilotLoader) Load(_ context.Context, episodeID uint, _ bool) (episodecopilot.EpisodeContext, error) {
	if episodeID != s.context.EpisodeID {
		return episodecopilot.EpisodeContext{EpisodeID: episodeID}, nil
	}
	return s.context, nil
}

type launchExecution struct {
	deltas []string
	result json.RawMessage
	delay  time.Duration
}

type launchRuntime struct {
	mu        sync.Mutex
	queue     []launchExecution
	fallback  launchExecution
	snapshots map[codexruntime.ExecutionID]launchExecution
	next      int
}

func newLaunchRuntime(queue ...launchExecution) *launchRuntime {
	fallback := launchExecution{}
	if len(queue) > 0 {
		fallback = queue[len(queue)-1]
	}
	return &launchRuntime{
		queue:     append([]launchExecution(nil), queue...),
		fallback:  fallback,
		snapshots: map[codexruntime.ExecutionID]launchExecution{},
	}
}

func (l *launchRuntime) CreateExecution(
	_ context.Context,
	request codexruntime.ExecutionRequest,
) (codexruntime.ExecutionSnapshot, error) {
	if strings.Contains(request.Prompt, "FORCE_FAIL") {
		return codexruntime.ExecutionSnapshot{}, fmt.Errorf("forced runtime failure")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if strings.Contains(string(request.OutputSchema), `"sufficient"`) {
		l.next++
		id := codexruntime.ExecutionID(fmt.Sprintf("launch-%d", l.next))
		l.snapshots[id] = launchExecution{result: json.RawMessage(`{"sufficient":true,"gaps":[]}`)}
		return codexruntime.ExecutionSnapshot{ID: id, Status: codexruntime.StatusRunning}, nil
	}
	next := l.fallback
	if len(l.queue) > 0 {
		next = l.queue[0]
		l.queue = l.queue[1:]
	} else if len(next.deltas) == 0 {
		return codexruntime.ExecutionSnapshot{}, fmt.Errorf("unexpected execution")
	}
	if strings.Contains(request.Prompt, "SLOW_HOLD") {
		next.delay = 8 * time.Second
	}
	l.next++
	id := codexruntime.ExecutionID(fmt.Sprintf("launch-%d", l.next))
	l.snapshots[id] = next
	return codexruntime.ExecutionSnapshot{ID: id, Status: codexruntime.StatusRunning}, nil
}

func (l *launchRuntime) SubscribeExecution(
	ctx context.Context,
	id codexruntime.ExecutionID,
) (<-chan codexruntime.Event, error) {
	l.mu.Lock()
	next := l.snapshots[id]
	l.mu.Unlock()
	events := make(chan codexruntime.Event, 8)
	go func() {
		defer close(events)
		if next.delay > 0 {
			timer := time.NewTimer(next.delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
		select {
		case <-ctx.Done():
			return
		case events <- codexruntime.Event{Type: codexruntime.EventStarted}:
		}
		for _, delta := range next.deltas {
			select {
			case <-ctx.Done():
				return
			case events <- codexruntime.Event{Type: codexruntime.EventOutputDelta, Text: delta}:
			}
		}
		select {
		case <-ctx.Done():
			return
		case events <- codexruntime.Event{Type: codexruntime.EventTerminal}:
		}
	}()
	return events, nil
}

func (l *launchRuntime) GetExecution(
	_ context.Context,
	id codexruntime.ExecutionID,
) (codexruntime.ExecutionSnapshot, error) {
	l.mu.Lock()
	next := l.snapshots[id]
	l.mu.Unlock()
	result := next.result
	if len(result) == 0 {
		result = json.RawMessage(`{"text":"ok"}`)
	}
	return codexruntime.ExecutionSnapshot{
		ID:     id,
		Status: codexruntime.StatusCompleted,
		Result: result,
	}, nil
}

func (l *launchRuntime) CancelExecution(
	context.Context,
	codexruntime.ExecutionID,
) (codexruntime.CancellationResult, error) {
	return codexruntime.CancellationResult{Status: codexruntime.StatusCancelled}, nil
}

func (l *launchRuntime) Close(context.Context) error { return nil }
