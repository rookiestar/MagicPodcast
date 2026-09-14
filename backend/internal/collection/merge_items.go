package collection

import (
	"fmt"
	"strings"
)

// mergeDraftItems runs once over validated source entries, before preview or
// refresh diffing. Recommendation identity is the entire source text, never a
// paragraph split of a previously merged value.
func mergeDraftItems(draft *Draft) (*Draft, error) {
	draft.SourceItemCount = len(draft.Items)
	items := make([]ItemDraft, 0, len(draft.Items))
	positions := make(map[string]int)
	podcastIDs := make(map[string]string)
	recommendations := make(map[string][]string)
	seenText := make(map[string]map[string]bool)
	for _, item := range draft.Items {
		id := item.ExternalEpisodeID
		pid := strings.TrimSpace(item.ExternalPodcastID)
		if previous := podcastIDs[id]; previous != "" && pid != "" && previous != pid {
			return nil, fmt.Errorf("%w: conflicting podcast identity for episode %q", ErrIncompleteSource, id)
		}
		if pid != "" {
			podcastIDs[id] = pid
		}
		if position, exists := positions[id]; exists {
			// Any restricted occurrence must not become playable through merging.
			if item.AudioURL == "" {
				items[position].AudioURL = ""
				items[position].AudioMimeType = ""
				items[position].AudioSize = 0
			}
			items[position].IsPrivateMedia = items[position].IsPrivateMedia || item.IsPrivateMedia
		} else {
			positions[id] = len(items)
			items = append(items, item)
			seenText[id] = make(map[string]bool)
		}
		text := strings.TrimSpace(item.Recommendation)
		if text != "" && !seenText[id][text] {
			seenText[id][text] = true
			recommendations[id] = append(recommendations[id], text)
		}
	}
	for index := range items {
		items[index].Recommendation = strings.Join(recommendations[items[index].ExternalEpisodeID], "\n\n")
	}
	draft.Items = items
	draft.DuplicateItemCount = draft.SourceItemCount - len(items)
	return draft, nil
}
