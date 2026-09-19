package services

import (
	"context"
	"database/sql"
	"fmt"
	"magicpodcast/internal/config"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// Search pins normalization functions to the connection executing this request.
// No schema or global driver registration is needed, including in temporary databases.
func (s *SearchService) Search(req SearchRequest) (*SearchResponse, error) {
	if !hasSearchHan(req.Query) {
		return s.search(req)
	}
	var result *SearchResponse
	err := s.db.Connection(func(db *gorm.DB) error {
		conn, ok := db.Statement.ConnPool.(*sql.Conn)
		if !ok {
			return fmt.Errorf("search requires a SQLite connection")
		}
		var registered int
		if err := conn.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM pragma_function_list WHERE name = 'search_score'").Scan(&registered); err != nil {
			return err
		}
		if registered == 0 {
			if err := conn.Raw(func(raw interface{}) error {
				sqlite, ok := raw.(*sqlite3.SQLiteConn)
				if !ok {
					return fmt.Errorf("search requires the SQLite driver")
				}
				return sqlite.RegisterFunc("search_score", func(title, author, body, keyword string, episode bool, titleWeight, authorWeight, bodyWeight, exact, prefix, contains, occurrence float64) float64 {
					cfg := config.SearchConfig{}
					cfg.Weights.PodcastTitle = titleWeight
					cfg.Weights.EpisodeTitle = titleWeight
					cfg.Weights.Author = authorWeight
					cfg.Weights.PodcastDesc = bodyWeight
					cfg.Weights.EpisodeContent = bodyWeight
					cfg.MatchMultipliers.Exact = exact
					cfg.MatchMultipliers.Prefix = prefix
					cfg.MatchMultipliers.Contains = contains
					cfg.MatchMultipliers.Occurrence = occurrence
					if episode {
						return calculateEpisodeRelevance(title, body, keyword, cfg)
					}
					return calculatePodcastRelevance(title, author, body, keyword, cfg)
				}, true)
			}); err != nil {
				return err
			}
		}

		scoped := *s
		scoped.db = db.Set("search.config", s.config).Session(&gorm.Session{NewDB: true})
		var err error
		result, err = scoped.search(req)
		return err
	})
	return result, err
}
