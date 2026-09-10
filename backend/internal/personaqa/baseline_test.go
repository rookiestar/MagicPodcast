package personaqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

func TestLoadBaselineReportsCoverageAndKeepsFixturesSynthetic(t *testing.T) {
	baseline, err := Load()
	require.NoError(t, err)

	require.Equal(t, PrivacySyntheticOnly, baseline.Samples.Privacy)
	require.Equal(t, StatusMissing, baseline.Samples.RealTranscribedEpisodes.Status)
	require.Zero(t, baseline.Samples.RealTranscribedEpisodes.Available)
	require.NotEmpty(t, baseline.Samples.RealTranscribedEpisodes.Reason)

	for _, key := range requiredCaseKeys {
		item, ok := baseline.CoverageCase(key)
		require.Truef(t, ok, "required case %s must appear in the coverage matrix", key)
		require.Equal(t, StatusPresent, item.Status, key)
		require.NotEmpty(t, item.SampleIDs, key)
		require.NotEmpty(t, item.QuestionIDs, key)
	}
	realCase, ok := baseline.CoverageCase("real-transcribed-episodes-10-20")
	require.True(t, ok)
	require.Equal(t, StatusMissing, realCase.Status)
	require.NotEmpty(t, realCase.Reason)

	require.NotEmpty(t, baseline.QuestionsByCategory(CategoryDirectEvidence))
	require.NotEmpty(t, baseline.QuestionsByCategory(CategoryConflict))
	require.NotEmpty(t, baseline.QuestionsByCategory(CategoryNoAnswer))

	conflict, ok := baseline.Question("q-conflict-overtime-history")
	require.True(t, ok)
	require.Equal(t, CategoryConflict, conflict.Category)
	require.Len(t, conflict.ExpectedFragmentIDs, 2)
	require.Equal(t, "sourced_original_keep_conflict", conflict.ExpectedAnswerKind)

	noAnswer, ok := baseline.Question("q-no-answer-quantum")
	require.True(t, ok)
	require.Empty(t, noAnswer.ExpectedFragmentIDs)
	require.Equal(t, "cannot_judge", noAnswer.ExpectedAnswerKind)
	require.True(t, noAnswer.NeedsWebSearch)

	paraphrase, ok := baseline.Question("q-direct-overtime-paraphrase")
	require.True(t, ok)
	require.True(t, paraphrase.Paraphrase)
	segment, ok := baseline.Segment(paraphrase.ExpectedFragmentIDs[0])
	require.True(t, ok)
	require.Contains(t, segment.Text, "不赞成无限制加班")
	require.NotContains(t, paraphrase.Question, "不赞成无限制加班")

	require.Contains(t, baseline.Coverage.FixedEvalGates, "must_not_merge_distinct_people")
	require.Contains(t, baseline.Coverage.FixedEvalGates, "must_not_cite_pending_or_stale_fragments_as_person_views")
	require.Contains(t, baseline.Coverage.FixedEvalGates, "must_not_emit_unmarked_inference")
	require.Contains(t, baseline.Metrics, "question_id | expected_kind")

	assertFixturesStaySynthetic(t, testdataDir())
}

func TestLoadBaselineRejectsUnknownJSONFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "samples.json"), []byte(`{"privacy":"synthetic-only","unexpected":true}`), 0o600))
	_, err := loadFrom(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func assertFixturesStaySynthetic(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	privateMarkers := []string{
		"lark.com",
		"feishu.cn",
		"minutes.feishu",
		"openid",
		"access_token",
		"BEGIN PRIVATE",
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		text := string(raw)
		for _, marker := range privateMarkers {
			require.NotContainsf(t, strings.ToLower(text), marker, "%s looks like private transcript material", entry.Name())
		}
		require.False(t, looksLikePhoneNumber(text), "%s contains a phone-like token", entry.Name())
	}
}

func looksLikePhoneNumber(text string) bool {
	digits := 0
	for _, r := range text {
		if unicode.IsDigit(r) {
			digits++
			if digits >= 11 {
				return true
			}
			continue
		}
		if r == '-' || r == ' ' {
			continue
		}
		digits = 0
	}
	return false
}
