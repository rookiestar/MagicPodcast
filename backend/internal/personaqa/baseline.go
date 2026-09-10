package personaqa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

const (
	PrivacySyntheticOnly = "synthetic-only"
	StatusPresent        = "present"
	StatusMissing        = "missing"

	CategoryDirectEvidence = "direct_evidence"
	CategoryConflict       = "conflict"
	CategoryNoAnswer       = "no_answer"
)

var requiredCaseKeys = []string{
	"same-name",
	"nickname",
	"cross-episode-role-change",
	"anonymous-speaker",
	"mixed-labels",
	"host-paraphrase",
	"negation-rhetorical",
	"chinese-paraphrase",
	"no-answer",
}

type Baseline struct {
	Samples   SamplesFile
	Questions QuestionsFile
	Coverage  CoverageFile
	Metrics   string
}

type SamplesFile struct {
	Privacy                 string                 `json:"privacy"`
	RealTranscribedEpisodes RealTranscriptCoverage `json:"real_transcribed_episodes"`
	Podcasts                []PodcastSample        `json:"podcasts"`
	People                  []PersonSample         `json:"people"`
	Episodes                []EpisodeSample        `json:"episodes"`
}

type RealTranscriptCoverage struct {
	Requested string `json:"requested"`
	Available int    `json:"available"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}

type PodcastSample struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

type PersonSample struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	Aliases      []string `json:"aliases"`
	IdentityNote string   `json:"identity_note"`
}

type EpisodeSample struct {
	ID                 string             `json:"id"`
	PodcastID          string             `json:"podcast_id"`
	Title              string             `json:"title"`
	PublishedDate      string             `json:"published_date"`
	ShowNotes          string             `json:"show_notes"`
	PrivateNotes       string             `json:"private_notes,omitempty"`
	TranscriptSegments []SegmentSample    `json:"transcript_segments"`
	Appearances        []AppearanceSample `json:"appearances"`
	MentionedOnly      []MentionSample    `json:"mentioned_only"`
}

type SegmentSample struct {
	ID                     string `json:"id"`
	Order                  int    `json:"order"`
	SpeakerLabel           string `json:"speaker_label"`
	StartMS                int64  `json:"start_ms"`
	Text                   string `json:"text"`
	AttributionPersonID    string `json:"attribution_person_id"`
	AttributionStatus      string `json:"attribution_status"`
	AttributionBasis       string `json:"attribution_basis"`
	ParaphraseOfPersonID   string `json:"paraphrase_of_person_id,omitempty"`
	Rhetorical             bool   `json:"rhetorical,omitempty"`
	TrueSpeakerIfCorrected string `json:"true_speaker_if_corrected,omitempty"`
}

type AppearanceSample struct {
	PersonID string `json:"person_id"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type MentionSample struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type QuestionsFile struct {
	Questions []QuestionSample `json:"questions"`
}

type QuestionSample struct {
	ID                  string   `json:"id"`
	Category            string   `json:"category"`
	EpisodeID           string   `json:"episode_id"`
	TargetPersonID      string   `json:"target_person_id"`
	Question            string   `json:"question"`
	Paraphrase          bool     `json:"paraphrase"`
	ExpectedFragmentIDs []string `json:"expected_fragment_ids"`
	AllowInference      bool     `json:"allow_inference"`
	NeedsWebSearch      bool     `json:"needs_web_search"`
	ForbiddenEvidence   []string `json:"forbidden_evidence"`
	ExpectedAnswerKind  string   `json:"expected_answer_kind"`
	Notes               string   `json:"notes,omitempty"`
}

type CoverageFile struct {
	RequiredCases      []CoverageCase      `json:"required_cases"`
	QuestionCategories map[string][]string `json:"question_categories"`
	FixedEvalGates     []string            `json:"fixed_eval_gates"`
}

type CoverageCase struct {
	Key         string   `json:"key"`
	Status      string   `json:"status"`
	SampleIDs   []string `json:"sample_ids"`
	PersonIDs   []string `json:"person_ids"`
	QuestionIDs []string `json:"question_ids"`
	Reason      string   `json:"reason,omitempty"`
}

var (
	loadOnce  sync.Once
	cached    Baseline
	cachedErr error
)

func testdataDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "testdata"
	}
	return filepath.Join(filepath.Dir(file), "testdata")
}

func Load() (Baseline, error) {
	loadOnce.Do(func() {
		cached, cachedErr = loadFrom(testdataDir())
	})
	return cached, cachedErr
}

func loadFrom(dir string) (Baseline, error) {
	var baseline Baseline
	if err := decodeJSONFile(filepath.Join(dir, "samples.json"), &baseline.Samples); err != nil {
		return Baseline{}, err
	}
	if err := decodeJSONFile(filepath.Join(dir, "questions.json"), &baseline.Questions); err != nil {
		return Baseline{}, err
	}
	if err := decodeJSONFile(filepath.Join(dir, "coverage.json"), &baseline.Coverage); err != nil {
		return Baseline{}, err
	}
	metrics, err := os.ReadFile(filepath.Join(dir, "metrics.md"))
	if err != nil {
		return Baseline{}, fmt.Errorf("read metrics recipe: %w", err)
	}
	baseline.Metrics = string(metrics)
	if err := baseline.validate(); err != nil {
		return Baseline{}, err
	}
	return baseline, nil
}

func decodeJSONFile(path string, dest any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

func (b Baseline) Person(id string) (PersonSample, bool) {
	for _, person := range b.Samples.People {
		if person.ID == id {
			return person, true
		}
	}
	return PersonSample{}, false
}

func (b Baseline) Episode(id string) (EpisodeSample, bool) {
	for _, episode := range b.Samples.Episodes {
		if episode.ID == id {
			return episode, true
		}
	}
	return EpisodeSample{}, false
}

func (b Baseline) Segment(id string) (SegmentSample, bool) {
	for _, episode := range b.Samples.Episodes {
		for _, segment := range episode.TranscriptSegments {
			if segment.ID == id {
				return segment, true
			}
		}
	}
	return SegmentSample{}, false
}

func (b Baseline) Question(id string) (QuestionSample, bool) {
	for _, question := range b.Questions.Questions {
		if question.ID == id {
			return question, true
		}
	}
	return QuestionSample{}, false
}

func (b Baseline) CoverageCase(key string) (CoverageCase, bool) {
	for _, item := range b.Coverage.RequiredCases {
		if item.Key == key {
			return item, true
		}
	}
	return CoverageCase{}, false
}

func (b Baseline) QuestionsByCategory(category string) []QuestionSample {
	out := make([]QuestionSample, 0)
	for _, question := range b.Questions.Questions {
		if question.Category == category {
			out = append(out, question)
		}
	}
	return out
}

func (b Baseline) validate() error {
	if b.Samples.Privacy != PrivacySyntheticOnly {
		return fmt.Errorf("samples privacy must be %s", PrivacySyntheticOnly)
	}
	if b.Samples.RealTranscribedEpisodes.Status != StatusMissing {
		return fmt.Errorf("real transcribed coverage must be marked missing until authorized transcripts exist")
	}
	if strings.TrimSpace(b.Samples.RealTranscribedEpisodes.Reason) == "" {
		return fmt.Errorf("missing real-transcript coverage must include a reason")
	}
	people := map[string]PersonSample{}
	for _, person := range b.Samples.People {
		if person.ID == "" || person.DisplayName == "" {
			return fmt.Errorf("person is missing id or display_name")
		}
		if _, exists := people[person.ID]; exists {
			return fmt.Errorf("duplicate person id %s", person.ID)
		}
		people[person.ID] = person
	}
	episodes := map[string]EpisodeSample{}
	segments := map[string]SegmentSample{}
	for _, episode := range b.Samples.Episodes {
		if episode.ID == "" || episode.ShowNotes == "" {
			return fmt.Errorf("episode %s is missing id or show notes", episode.ID)
		}
		if _, exists := episodes[episode.ID]; exists {
			return fmt.Errorf("duplicate episode id %s", episode.ID)
		}
		episodes[episode.ID] = episode
		seenOrder := map[int]struct{}{}
		for _, segment := range episode.TranscriptSegments {
			if segment.ID == "" || strings.TrimSpace(segment.Text) == "" {
				return fmt.Errorf("episode %s has an incomplete segment", episode.ID)
			}
			if _, exists := segments[segment.ID]; exists {
				return fmt.Errorf("duplicate segment id %s", segment.ID)
			}
			if _, exists := seenOrder[segment.Order]; exists {
				return fmt.Errorf("episode %s has duplicate segment order %d", episode.ID, segment.Order)
			}
			seenOrder[segment.Order] = struct{}{}
			if segment.AttributionPersonID != "" {
				if _, ok := people[segment.AttributionPersonID]; !ok {
					return fmt.Errorf("segment %s references unknown person %s", segment.ID, segment.AttributionPersonID)
				}
			}
			segments[segment.ID] = segment
		}
		for _, appearance := range episode.Appearances {
			if _, ok := people[appearance.PersonID]; !ok {
				return fmt.Errorf("episode %s appearance references unknown person %s", episode.ID, appearance.PersonID)
			}
		}
	}
	questions := map[string]QuestionSample{}
	for _, question := range b.Questions.Questions {
		if question.ID == "" || question.Question == "" {
			return fmt.Errorf("question is missing id or text")
		}
		if _, exists := questions[question.ID]; exists {
			return fmt.Errorf("duplicate question id %s", question.ID)
		}
		switch question.Category {
		case CategoryDirectEvidence, CategoryConflict, CategoryNoAnswer:
		default:
			return fmt.Errorf("question %s has unknown category %s", question.ID, question.Category)
		}
		if question.EpisodeID != "" {
			if _, ok := episodes[question.EpisodeID]; !ok {
				return fmt.Errorf("question %s references unknown episode %s", question.ID, question.EpisodeID)
			}
		}
		if question.TargetPersonID != "" {
			if _, ok := people[question.TargetPersonID]; !ok {
				return fmt.Errorf("question %s references unknown person %s", question.ID, question.TargetPersonID)
			}
		}
		for _, fragmentID := range question.ExpectedFragmentIDs {
			if _, ok := segments[fragmentID]; !ok {
				return fmt.Errorf("question %s expected fragment %s is missing", question.ID, fragmentID)
			}
		}
		questions[question.ID] = question
	}
	if len(b.QuestionsByCategory(CategoryDirectEvidence)) == 0 ||
		len(b.QuestionsByCategory(CategoryConflict)) == 0 ||
		len(b.QuestionsByCategory(CategoryNoAnswer)) == 0 {
		return fmt.Errorf("baseline must include direct_evidence, conflict, and no_answer questions")
	}
	for _, key := range requiredCaseKeys {
		item, ok := b.CoverageCase(key)
		if !ok {
			return fmt.Errorf("coverage matrix missing required case %s", key)
		}
		if item.Status != StatusPresent && item.Status != StatusMissing {
			return fmt.Errorf("coverage case %s has invalid status %s", key, item.Status)
		}
		if item.Status == StatusMissing && strings.TrimSpace(item.Reason) == "" {
			return fmt.Errorf("coverage case %s is missing a reason", key)
		}
		if item.Status == StatusPresent {
			if len(item.SampleIDs) == 0 || len(item.QuestionIDs) == 0 {
				return fmt.Errorf("present coverage case %s needs samples and questions", key)
			}
			for _, sampleID := range item.SampleIDs {
				if _, ok := episodes[sampleID]; !ok {
					return fmt.Errorf("coverage case %s references unknown sample %s", key, sampleID)
				}
			}
			for _, questionID := range item.QuestionIDs {
				if _, ok := questions[questionID]; !ok {
					return fmt.Errorf("coverage case %s references unknown question %s", key, questionID)
				}
			}
		}
	}
	if !strings.Contains(b.Metrics, "first_status_ms") || !strings.Contains(b.Metrics, "usable_answer_ms") {
		return fmt.Errorf("metrics recipe must define first-status and usable-answer timing fields")
	}
	return nil
}

func ContainsPrivateTranscriptMarkers(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "@") && strings.Contains(trimmed, ".") {
		return true
	}
	digits := 0
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return false
}
