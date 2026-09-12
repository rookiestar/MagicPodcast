package personidentity

import (
	"context"
	"encoding/json"
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
		// Synthetic QA fixtures supply declared decisions, never a production
		// fallback that infers identity without Runtime.
		preparer := service
		if service.suggester == nil {
			var suggested baselineCandidates
			for _, appearance := range episode.Appearances {
				for _, person := range baseline.Samples.People {
					if person.ID != appearance.PersonID {
						continue
					}
					item := SuggestedCandidate{DisplayName: person.DisplayName, Aliases: person.Aliases, IdentityNote: person.IdentityNote, Role: appearance.Role, Status: appearance.Status, EvidenceKind: "verified_runtime", EvidenceLocator: appearance.Evidence}
					// Baseline fixture person IDs explicitly label cross-episode identity.
					proof, _ := json.Marshal(identityProposal{NameType: "canonical", IdentityAnchor: identityAnchor{Kind: "distinctive_affiliation", Key: "synthetic:" + person.ID}})
					item.EvidenceLocator = string(proof)
					for _, fragment := range episode.TranscriptSegments {
						if fragment.AttributionPersonID == person.ID && fragment.AttributionStatus == StatusConfirmed {
							item.SpeechOrders = append(item.SpeechOrders, fragment.Order)
						}
					}
					suggested = append(suggested, item)
				}
			}
			copy := *service
			copy.suggester = suggested
			preparer = &copy
		}
		prepared, err := preparer.Prepare(ctx, EpisodeSources{
			EpisodeID:     row.ID,
			ShowNotes:     episode.ShowNotes,
			SourceKind:    SourceTranscript,
			SourceVersion: "fixture-" + episode.ID,
			Segments:      segments,
		})
		if err != nil {
			return SeededLibrary{}, err
		}
		if prepared.Draft != nil {
			for i := range prepared.Draft.Matches {
				prepared.Draft.Matches[i].Selected = prepared.Draft.Matches[i].SuggestedStatus == StatusConfirmed
			}

			_, err = preparer.Review(ctx, row.ID, ReviewRequest{DraftID: prepared.Draft.ID, Revision: prepared.Revision, SourceVersion: prepared.Draft.SourceVersion, Matches: prepared.Draft.Matches}, true)
			if err != nil {
				return SeededLibrary{}, err
			}
			for _, m := range prepared.Draft.Matches {
				if m.SuggestedStatus != StatusPending {
					continue
				}
				person := models.Person{StableKey: newStableKey(), DisplayName: m.DisplayName, IdentityNote: m.IdentityNote, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
				if err := db.Create(&person).Error; err != nil {
					return SeededLibrary{}, err
				}
				if err := db.Create(&models.EpisodeAppearance{EpisodeID: row.ID, PersonID: person.ID, SourceVersion: prepared.Draft.SourceVersion, Role: m.Role, Status: StatusPending, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}).Error; err != nil {
					return SeededLibrary{}, err
				}
			}
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

type baselineCandidates []SuggestedCandidate

func (c baselineCandidates) Suggest(context.Context, EpisodeSources) ([]SuggestedCandidate, error) {
	return c, nil
}
