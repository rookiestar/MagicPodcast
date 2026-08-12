package services

import (
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

type TriageService struct {
	consumption *ConsumptionService
}

func NewTriageService(db *gorm.DB) *TriageService {
	return &TriageService{
		consumption: NewConsumptionService(db),
	}
}

func (s *TriageService) Consumption() *ConsumptionService {
	return s.consumption
}

func (s *TriageService) DecisionsForEpisodes(episodeIDs []uint) (map[uint]models.EpisodeTriageDecision, error) {
	return s.consumption.StatesForEpisodes(episodeIDs)
}
