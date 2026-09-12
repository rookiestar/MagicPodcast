package handlers_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"
	"magicpodcast/internal/personidentity"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type stagedIdentityFixture struct {
	release <-chan struct{}
	fail    bool
}

func (f stagedIdentityFixture) Suggest(ctx context.Context, _ personidentity.EpisodeSources) ([]personidentity.SuggestedCandidate, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.release:
	}
	if f.fail {
		return nil, errors.New("private runtime diagnostics must not be exposed")
	}
	return fixedIdentityFixture{{DisplayName: "林言", Role: "host", Status: "confirmed", EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}, nil
}

type streamingPeopleFixture struct {
	*personidentity.Service
	source    personidentity.EpisodeSources
	committed chan struct{}
}

func (f streamingPeopleFixture) PrepareCurrent(ctx context.Context, _ uint) (personidentity.EpisodePeople, error) {
	result, err := f.Prepare(ctx, f.source)
	if err == nil && f.committed != nil {
		close(f.committed)
		<-ctx.Done() // Commit succeeded, but the client loses the completion event.
	}
	return result, err
}

func TestPersonPreparationStreamsBeforePersistenceAndReturnsSavedDraft(t *testing.T) {
	for _, failure := range []string{"", "runtime", "save", "cancel_after_commit"} {
		t.Run(failure, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db := openIsolatedPersonDB(t)
			pod := models.Podcast{XYZID: "stream", Title: "访谈", FeedURL: "https://example.test/stream"}
			require.NoError(t, db.Create(&pod).Error)
			ep := models.Episode{PodcastID: pod.ID, GUID: "stream", Title: "访谈"}
			require.NoError(t, db.Create(&ep).Error)
			release := make(chan struct{})
			service, err := personidentity.NewService(db, stagedIdentityFixture{release: release, fail: failure == "runtime"})
			require.NoError(t, err)
			fixture := streamingPeopleFixture{Service: service, source: personidentity.EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []personidentity.Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "我是林言。"}}}}
			if failure == "cancel_after_commit" {
				fixture.committed = make(chan struct{})
			}
			router := gin.New()
			router.POST("/episodes/:id/people/prepare", handlers.NewPersonHandler(fixture).Prepare)
			server := httptest.NewServer(router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/episodes/%d/people/prepare", server.URL, ep.ID), bytes.NewBufferString("{}"))
			require.NoError(t, err)
			request.Header.Set("Accept", "text/event-stream")
			response, err := server.Client().Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
			reader := bufio.NewReader(response.Body)
			first, err := reader.ReadString('\n')
			require.NoError(t, err)
			require.Contains(t, first, `"stage":"identify"`)
			require.Contains(t, first, `"source_version":"v1"`)
			// Work cannot finish before release. Receiving this event proves HTTP flush.
			before, err := service.ListEpisodePeople(ctx, ep.ID)
			require.NoError(t, err)
			require.Nil(t, before.Draft)
			if failure == "save" {
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_draft_save", func(tx *gorm.DB) {
					if tx.Statement.Table == "person_drafts" {
						tx.AddError(errors.New("private database failure"))
					}
				}))
			}
			close(release)
			if fixture.committed != nil {
				select {
				case <-fixture.committed:
				case <-ctx.Done():
					t.Fatal("draft did not commit")
				}
				cancel()
				tail, _ := io.ReadAll(reader)
				require.NotContains(t, string(tail), `"type":"complete"`)
				persisted, err := service.ListEpisodePeople(context.Background(), ep.ID)
				require.NoError(t, err)
				require.NotNil(t, persisted.Draft, "cancellation must not undo a committed draft")
				require.Empty(t, persisted.People)
				return
			}
			tail, err := io.ReadAll(reader)
			require.NoError(t, err)
			persisted, err := service.ListEpisodePeople(ctx, ep.ID)
			require.NoError(t, err)
			if failure != "" {
				require.Contains(t, string(tail), `"type":"error"`)
				require.NotContains(t, string(tail), `"type":"complete"`)
				require.NotContains(t, string(tail), "private runtime")
				require.Nil(t, persisted.Draft)
				if failure == "save" {
					require.Contains(t, string(tail), `"stage":"save"`)
				}
				return
			}
			require.NotNil(t, persisted.Draft)
			require.Contains(t, string(tail), `"stage":"save"`)
			require.Less(t, strings.Index(string(tail), `"stage":"save"`), strings.Index(string(tail), `"type":"complete"`))
			var terminal struct {
				Data      personidentity.EpisodePeople `json:"data"`
				RequestID string                       `json:"request_id"`
			}
			for _, line := range strings.Split(string(tail), "\n") {
				if strings.Contains(line, `"type":"complete"`) {
					require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &terminal))
				}
			}
			require.Equal(t, persisted.Draft.ID, terminal.Data.Draft.ID)
			require.Equal(t, persisted.Revision, terminal.Data.Revision)
			require.NotEmpty(t, terminal.RequestID)
			require.Contains(t, first, terminal.RequestID)
			require.Empty(t, persisted.People, "preparation must not confer approved persona eligibility")
		})
	}
}
