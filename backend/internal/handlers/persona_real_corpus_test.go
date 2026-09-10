package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
)

// The corpus is deliberately external: never embed personal transcripts in Git.
func TestPersonaRealCorpus(t *testing.T) {
	input := os.Getenv("PERSONA_REAL_CORPUS")
	if input == "" {
		t.Skip("set PERSONA_REAL_CORPUS to an authorized read-only corpus export")
	}
	output := os.Getenv("PERSONA_LAUNCH_LOG_DIR")
	require.NotEmpty(t, output)
	require.NoError(t, os.MkdirAll(output, 0700))
	var sources []struct {
		EpisodeID     uint   `json:"episode_id"`
		Title         string `json:"title"`
		ShowNotes     string `json:"show_notes"`
		PodcastTitle  string `json:"podcast_title"`
		PublishedDate string `json:"published_date"`
		Timeline      struct {
			Segments []processing.TranscriptSegment `json:"segments"`
		} `json:"timeline"`
	}
	data, err := os.ReadFile(input)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &sources))
	_, here, _, _ := runtime.Caller(0)
	_, err = config.Load(filepath.Join(filepath.Dir(here), "..", "..", "configs", "config.example.yaml"))
	require.NoError(t, err)
	python := filepath.Join(os.Getenv("HOME"), ".codex", "venv-openai-codex-0.147.0", "bin", "python")
	workRoot := t.TempDir()
	host, err := codexruntime.NewProcessHost(codexruntime.ProcessHostConfig{Command: []string{python, filepath.Join(filepath.Dir(here), "..", "codexruntime", "runtime_host.py")}, WorkRoot: workRoot, Profiles: codexruntime.DefaultProfiles(), Environment: map[string]string{"PATH": filepath.Dir(python) + string(os.PathListSeparator) + os.Getenv("PATH")}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close(context.Background()) })
	db := openLaunchDB(t)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	var suggester personidentity.CandidateSuggester = personidentity.NewRuntimeSuggester(corpusCaptureRuntime{Runtime: host, dir: output}, workRoot)
	if replay := os.Getenv("PERSONA_IDENTITIES_REPLAY"); replay != "" {
		suggester = corpusIdentityReplay{dir: replay}
	}
	people, err := personidentity.NewService(db, suggester, search)
	require.NoError(t, err)
	for _, source := range sources {
		t.Run(fmt.Sprint(source.EpisodeID), func(t *testing.T) {
			pod := models.Podcast{XYZID: fmt.Sprint(source.EpisodeID), Title: source.PodcastTitle, FeedURL: fmt.Sprintf("https://example.test/%d", source.EpisodeID)}
			require.NoError(t, db.Create(&pod).Error)
			published, _ := time.Parse("2006-01-02", source.PublishedDate[:10])
			ep := models.Episode{PodcastID: pod.ID, Title: source.Title, GUID: fmt.Sprint(source.EpisodeID), ShowNotes: source.ShowNotes, PublishedDate: published}
			ep.ID = source.EpisodeID
			require.NoError(t, db.Create(&ep).Error)
			start := time.Now()
			listed, err := people.Prepare(context.Background(), personidentity.EpisodeSources{EpisodeID: ep.ID, SourceVersion: fmt.Sprintf("real-eval-%d", ep.ID), ShowNotes: ep.ShowNotes, Segments: personidentity.SegmentsFromTranscript(source.Timeline.Segments)})
			require.NoError(t, err)
			report := struct {
				Seconds float64                      `json:"seconds"`
				People  personidentity.EpisodePeople `json:"result"`
			}{time.Since(start).Seconds(), listed}
			raw, err := json.MarshalIndent(report, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(output, fmt.Sprintf("real-%d.json", ep.ID)), raw, 0600))
			t.Logf("episode=%d people=%d fragments=%d seconds=%.1f", ep.ID, len(listed.People), len(listed.Attributions), report.Seconds)
		})
	}
	if t.Failed() || os.Getenv("PERSONA_REAL_ASK") != "1" {
		return
	}
	copilot, err := episodecopilot.NewService(&baselineCopilotLoader{db: db, people: people}, host, workRoot, episodecopilot.WithLibrary(people, search))
	require.NoError(t, err)
	database.SetTestDB(db)
	t.Cleanup(func() { database.SetTestDB(nil) })
	server := httptest.NewServer(router.SetupRouter(router.WithEpisodeCopilotModule(copilot), router.WithPersonIdentityModule(people)))
	t.Cleanup(server.Close)
	for _, q := range []struct {
		Episode  uint
		Name     string
		Question string
	}{{66252, "Kevin", "你为什么认为 RL as a Service 只是起点？"}, {66224, "雨鑫", "一个人训练模型的门槛到底在哪里？"}, {66314, "曾鸣", "为什么你认为科层制公司可能消亡？"}} {
		listed, err := people.ListEpisodePeople(context.Background(), q.Episode)
		require.NoError(t, err)
		var id uint
		for _, p := range listed.People {
			if strings.Contains(strings.ToLower(p.DisplayName), strings.ToLower(q.Name)) {
				id = p.ID
				break
			}
			for _, a := range p.Aliases {
				if strings.Contains(strings.ToLower(a), strings.ToLower(q.Name)) {
					id = p.ID
					break
				}
			}
		}
		if id == 0 {
			t.Errorf("expected source-backed participant %s not found", q.Name)
			continue
		}
		body, _ := json.Marshal(map[string]any{"question": q.Question, "target_person_id": id})
		answer := launchPOSTSSETimeout(t, server.URL+fmt.Sprintf("/api/v1/episodes/%d/copilot/questions", q.Episode), string(body), fmt.Sprintf("real-answer-%d", q.Episode), 5*time.Minute)
		require.Contains(t, answer, episodecopilotDisclaimer())
		require.Contains(t, answer, "库内 S")
		require.NotContains(t, answer, `"type":"error"`)
	}
}

type corpusCaptureRuntime struct {
	codexruntime.Runtime
	dir string
}

func (r corpusCaptureRuntime) GetExecution(ctx context.Context, id codexruntime.ExecutionID) (codexruntime.ExecutionSnapshot, error) {
	snapshot, err := r.Runtime.GetExecution(ctx, id)
	if err == nil && snapshot.Status == codexruntime.StatusCompleted {
		if e := os.WriteFile(filepath.Join(r.dir, "extraction-"+string(id)+".json"), snapshot.Result, 0600); e != nil {
			return snapshot, e
		}
	}
	return snapshot, err
}

// Explicit replay of an earlier live extraction isolates answer changes without
// repeating eight model calls. This is not reported as fresh identity inference.
type corpusIdentityReplay struct{ dir string }

func (r corpusIdentityReplay) Suggest(_ context.Context, source personidentity.EpisodeSources) ([]personidentity.SuggestedCandidate, error) {
	raw, err := os.ReadFile(filepath.Join(r.dir, fmt.Sprintf("real-%d.json", source.EpisodeID)))
	if err != nil {
		return nil, err
	}
	var captured struct {
		Result personidentity.EpisodePeople `json:"result"`
	}
	if err := json.Unmarshal(raw, &captured); err != nil {
		return nil, err
	}
	var result []personidentity.SuggestedCandidate
	for _, p := range captured.Result.People {
		candidate := personidentity.SuggestedCandidate{DisplayName: p.DisplayName, Aliases: p.Aliases, IdentityNote: p.IdentityNote, Role: p.Role, EvidenceKind: p.EvidenceKind, EvidenceLocator: p.EvidenceLocator}
		for _, a := range captured.Result.Attributions {
			if a.PersonID != nil && *a.PersonID == p.ID && a.Status == "confirmed" {
				candidate.SpeechOrders = append(candidate.SpeechOrders, a.FragmentOrder)
			}
		}
		result = append(result, candidate)
	}
	return result, nil
}

func TestPersonaPublicWebCapability(t *testing.T) {
	if os.Getenv("PERSONA_PUBLIC_WEB_E2E") != "1" {
		t.Skip("requires live public web capability")
	}
	_, here, _, _ := runtime.Caller(0)
	python := filepath.Join(os.Getenv("HOME"), ".codex", "venv-openai-codex-0.147.0", "bin", "python")
	root := t.TempDir()
	host, err := codexruntime.NewProcessHost(codexruntime.ProcessHostConfig{Command: []string{python, filepath.Join(filepath.Dir(here), "..", "codexruntime", "runtime_host.py")}, WorkRoot: root, Profiles: codexruntime.DefaultProfiles(), Environment: map[string]string{"PATH": filepath.Dir(python) + string(os.PathListSeparator) + os.Getenv("PATH")}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	workDir, err := os.MkdirTemp(root, "web-")
	require.NoError(t, err)
	workDir, err = filepath.EvalSymlinks(workDir)
	require.NoError(t, err)
	request := codexruntime.ExecutionRequest{Kind: codexruntime.ExecutionKindAssistant, WorkingDirectory: workDir, ToolRestriction: &codexruntime.ToolRestriction{Allowed: []codexruntime.ToolCapability{codexruntime.ToolWebSearch}}, Prompt: "Use the available web search tool to search for Paul Graham Maker's Schedule Manager's Schedule and actually open https://paulgraham.com/makersschedule.html. Return a verbatim sentence about programmers needing large units of time. Do not answer from memory. Report unable if no tool is available. Do not use shell, filesystem, skills, MCP or any other tool.", OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"opened":{"type":"boolean"},"url":{"type":"string"},"quote":{"type":"string"}},"required":["opened","url","quote"]}`)}
	snapshot, err := host.CreateExecution(ctx, request)
	require.NoError(t, err)
	stream, err := host.SubscribeExecution(ctx, snapshot.ID)
	require.NoError(t, err)
	var activities []codexruntime.Event
	for e := range stream {
		activities = append(activities, e)
	}
	final, err := host.GetExecution(ctx, snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, codexruntime.StatusCompleted, final.Status)
	output := os.Getenv("PERSONA_LAUNCH_LOG_DIR")
	if output != "" {
		require.NoError(t, os.MkdirAll(output, 0700))
		raw, _ := json.MarshalIndent(struct {
			Final  codexruntime.ExecutionSnapshot
			Events []codexruntime.Event
		}{final, activities}, "", "  ")
		require.NoError(t, os.WriteFile(filepath.Join(output, "web-capability.json"), raw, 0600))
	}
	completedWeb := 0
	for _, event := range activities {
		if event.Progress != nil && event.Progress.Category == codexruntime.CategoryWebSearch && event.Progress.State == codexruntime.ProgressCompleted {
			completedWeb++
		}
	}
	require.GreaterOrEqual(t, completedWeb, 1, "must emit actual web activity, not answer from memory")
	t.Log(string(final.Result))
	var result struct {
		Opened bool   `json:"opened"`
		URL    string `json:"url"`
		Quote  string `json:"quote"`
	}
	require.NoError(t, json.Unmarshal(final.Result, &result))
	require.True(t, result.Opened)
	require.Contains(t, result.URL, "paulgraham.com/makersschedule.html")
	require.NotEmpty(t, result.Quote)
	// Verify the complete persona API path with a clearly synthetic episode,
	// requiring an actual public original rather than library evidence.
	_, err = config.Load(filepath.Join(filepath.Dir(here), "..", "..", "configs", "config.example.yaml"))
	require.NoError(t, err)
	db := openLaunchDB(t)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	people, err := personidentity.NewService(db, nil, search)
	require.NoError(t, err)
	podcast := models.Podcast{XYZID: "public-web-check", Title: "公开搜索隔离测试", FeedURL: "https://example.test/web-check"}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{PodcastID: podcast.ID, Title: "公开原文问答能力测试（合成单集）", GUID: "public-web-check", ShowNotes: "嘉宾PaulGraham（Y Combinator联合创始人、文章作者）"}
	require.NoError(t, db.Create(&episode).Error)
	listed, err := people.Prepare(context.Background(), personidentity.EpisodeSources{EpisodeID: episode.ID, SourceVersion: "web-check-v1", ShowNotes: episode.ShowNotes, Segments: []personidentity.Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "这是一条隔离验证记录，没有人物观点。"}}})
	require.NoError(t, err)
	require.Len(t, listed.People, 1)
	id := listed.People[0].ID
	_, err = people.CorrectName(context.Background(), episode.ID, personidentity.NameCorrection{PersonID: id, DisplayName: "Paul Graham"})
	require.NoError(t, err)
	copilot, err := episodecopilot.NewService(&baselineCopilotLoader{db: db, people: people}, host, root, episodecopilot.WithLibrary(people, search))
	require.NoError(t, err)
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	server := httptest.NewServer(router.SetupRouter(router.WithEpisodeCopilotModule(copilot), router.WithPersonIdentityModule(people)))
	defer server.Close()
	body, _ := json.Marshal(map[string]any{"question": "请根据你在 paulgraham.com 的 Maker's Schedule, Manager's Schedule 原文解释：为什么程序员不适合把工作分成一小时的单位？", "target_person_id": id})
	answer := launchPOSTSSETimeout(t, server.URL+fmt.Sprintf("/api/v1/episodes/%d/copilot/questions", episode.ID), string(body), "public-persona-answer", 4*time.Minute)
	require.NotContains(t, answer, `"type":"error"`)
	require.Contains(t, answer, "[E1]")
	require.Contains(t, answer, "paulgraham.com/makersschedule")
	require.Contains(t, answer, episodecopilotDisclaimer())
}
