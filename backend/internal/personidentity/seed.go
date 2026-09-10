package personidentity

import (
	"context"
	"fmt"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/personaqa"

	"gorm.io/gorm"
)

type SeededLibrary struct {
	EpisodeIDs map[string]uint
	PeopleIDs  map[string]uint
}

func SeedBaseline(ctx context.Context, db *gorm.DB, service *Service) (SeededLibrary, error) {
	baseline, err := personaqa.Load()
	if err != nil {
		return SeededLibrary{}, err
	}
	podcastIDs := map[string]uint{}
	for _, podcast := range baseline.Samples.Podcasts {
		row := models.Podcast{
			XYZID:        podcast.ID,
			Title:        podcast.Title,
			Author:       podcast.Author,
			FeedURL:      fmt.Sprintf("https://example.test/%s.xml", podcast.ID),
			PodcastGUID:  podcast.ID,
			IsSubscribed: true,
		}
		if err := db.Create(&row).Error; err != nil {
			return SeededLibrary{}, err
		}
		podcastIDs[podcast.ID] = row.ID
	}
	episodeIDs := map[string]uint{}
	for _, episode := range baseline.Samples.Episodes {
		published, err := time.Parse("2006-01-02", episode.PublishedDate)
		if err != nil {
			return SeededLibrary{}, err
		}
		row := models.Episode{
			PodcastID:     podcastIDs[episode.PodcastID],
			Title:         episode.Title,
			ShowNotes:     episode.ShowNotes,
			Notes:         episode.PrivateNotes,
			PublishedDate: published,
			GUID:          episode.ID,
		}
		if err := db.Create(&row).Error; err != nil {
			return SeededLibrary{}, err
		}
		episodeIDs[episode.ID] = row.ID
		segments := make([]Segment, 0, len(episode.TranscriptSegments))
		for _, segment := range episode.TranscriptSegments {
			segments = append(segments, Segment{
				Order:        segment.Order,
				SpeakerLabel: segment.SpeakerLabel,
				StartMS:      segment.StartMS,
				Text:         segment.Text,
			})
		}
		if _, err := service.Prepare(ctx, EpisodeSources{
			EpisodeID:     row.ID,
			ShowNotes:     episode.ShowNotes,
			SourceKind:    SourceTranscript,
			SourceVersion: "artifact-" + episode.ID,
			Segments:      segments,
		}); err != nil {
			return SeededLibrary{}, err
		}
	}
	peopleIDs := map[string]uint{}
	var people []models.Person
	if err := db.Find(&people).Error; err != nil {
		return SeededLibrary{}, err
	}
	for _, person := range people {
		for _, sample := range baseline.Samples.People {
			if person.DisplayName == sample.DisplayName &&
				(person.IdentityNote == sample.IdentityNote ||
					(sample.IdentityNote != "" && person.IdentityNote != "" &&
						len(person.IdentityNote) > 0)) {
				if _, exists := peopleIDs[sample.ID]; !exists {
					peopleIDs[sample.ID] = person.ID
				}
			}
		}
	}
	return SeededLibrary{EpisodeIDs: episodeIDs, PeopleIDs: peopleIDs}, nil
}
