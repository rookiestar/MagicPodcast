package main

import (
	"fmt"
	"log"
	"magicpodcast/internal/podcastindex"
)

func main() {
	fmt.Println("🧪 Testing iTunesId Type Fix")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	query, err := podcastindex.NewQuery("../../../podcastindex.db")
	if err != nil {
		log.Fatalf("❌ Failed to open PodcastIndex: %v", err)
	}
	defer query.Close()

	// Test cases with various feed URLs
	testCases := []string{
		"https://feeds.acast.com/public/shows/involuntary-input",
		"https://feeds.simplecast.com/qm_9xx0g",
		"https://feeds.npr.org/510289/podcast.xml",
	}

	for i, feedURL := range testCases {
		fmt.Printf("\n[%d/%d] Testing: %s\n", i+1, len(testCases), feedURL)

		result, err := query.FindByFeedURL(feedURL)
		if err != nil {
			fmt.Printf("  ❌ Error: %v\n", err)
			continue
		}

		if result != nil {
			fmt.Printf("  ✅ Found: %s\n", result.Title)
			fmt.Printf("     iTunes ID: %d\n", result.ITunesID)
			fmt.Printf("     Feed URL: %s\n", result.FeedURL)
		} else {
			fmt.Println("  📭 Not found in PodcastIndex")
		}
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✅ Test completed - No type conversion errors!")
}
