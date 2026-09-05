package originallink

import "testing"

func TestResolvePrefersUsableStandardRSSLink(t *testing.T) {
	decision := Resolve(Input{
		Feed:         FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
		RSSLink:      "https://example.com/episode/1",
		GUID:         "https://hosting.wavpub.cn/pie/ep229/",
		ExistingLink: "https://old.example.com/episode/1",
	})
	if decision.Source != SourceRSSLink || decision.URL != "https://example.com/episode/1" {
		t.Fatalf("Resolve() = %+v, want the standard RSS link to win", decision)
	}
}

func TestResolveAcceptsBothLinkSchemesForStandardLinks(t *testing.T) {
	for _, link := range []string{
		"https://www.xiaoyuzhoufm.com/episode/abc",
		"http://example.com/episode/1",
	} {
		decision := Resolve(Input{RSSLink: link})
		if decision.Source != SourceRSSLink || decision.URL != link {
			t.Fatalf("Resolve(%q) = %+v, want rss_link", link, decision)
		}
	}
}

func TestResolveKeepsExistingLinkWhenFeedLinkMissing(t *testing.T) {
	decision := Resolve(Input{
		Feed:         FeedIdentity{FeedURL: "https://rss.art19.com/show"},
		GUID:         "plain-guid",
		ExistingLink: "https://example.com/episode/1",
	})
	if decision.Source != SourceExisting || decision.URL != "https://example.com/episode/1" {
		t.Fatalf("Resolve() = %+v, want the existing link to be kept", decision)
	}
}

func TestResolveReportsNoCandidateForEmptyHistory(t *testing.T) {
	decision := Resolve(Input{
		Feed:    FeedIdentity{FeedURL: "https://rss.art19.com/show"},
		GUID:    "plain-guid",
		RSSLink: "   ",
	})
	if decision.Source != SourceNone || decision.URL != "" {
		t.Fatalf("Resolve() = %+v, want an empty none decision", decision)
	}
}

func TestResolveNeverTreatsURLGUIDAsLinkOutsideWavPubRule(t *testing.T) {
	for _, feedURL := range []string{
		"https://rss.art19.com/show",
		"https://media.meldingcloud.com/rss",
		"https://www.lizhi.fm/rss/1.xml",
		"https://example.libsyn.com/rss",
		"https://evil.example.com/rss",
	} {
		decision := Resolve(Input{
			Feed: FeedIdentity{FeedURL: feedURL},
			GUID: "https://hosting.wavpub.cn/pie/ep229/",
		})
		if decision.Source != SourceNone || decision.URL != "" {
			t.Fatalf("Resolve(feed %q) = %+v, want no URL-GUID fallback", feedURL, decision)
		}
	}
}

func TestResolveWavPubFallbackRequiresStrictlyMatchingPageGUID(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{
			name: "verified feed with page GUID",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "https://hosting.wavpub.cn/pie/ep229/",
			},
			want: "https://hosting.wavpub.cn/pie/ep229/",
		},
		{
			name: "http GUID is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "http://hosting.wavpub.cn/pie/ep229/",
			},
			want: "",
		},
		{
			name: "wrong host is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "https://evil.wronghost.cn/pie/ep229/",
			},
			want: "",
		},
		{
			name: "lookalike host is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "https://hosting.wavpub.cn.evil.example.com/pie/ep229/",
			},
			want: "",
		},
		{
			name: "site root is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "https://hosting.wavpub.cn/",
			},
			want: "",
		},
		{
			name: "root query permalink is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "https://hosting.wavpub.cn/?p=822",
			},
			want: "",
		},
		{
			name: "relative GUID is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "/pie/ep229/",
			},
			want: "",
		},
		{
			name: "plain text GUID is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "ep229",
			},
			want: "",
		},
		{
			name: "enclosure media URL is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "https://cdn2.wavpub.com/hosting.wavpub.cn/pie-ep229.mp3",
			},
			want: "",
		},
		{
			name: "javascript GUID is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "javascript:alert(1)",
			},
			want: "",
		},
		{
			name: "data GUID is rejected",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
				GUID: "data:text/html,<script>alert(1)</script>",
			},
			want: "",
		},
		{
			name: "feed URL on another host cannot enable the rule",
			in: Input{
				Feed: FeedIdentity{FeedURL: "https://hosting.wavpub.cn.evil.example.com/pie/feed/"},
				GUID: "https://hosting.wavpub.cn/pie/ep229/",
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := Resolve(tc.in)
			if decision.URL != tc.want {
				t.Fatalf("Resolve() = %+v, want URL %q", decision, tc.want)
			}
			if tc.want != "" && decision.Source != SourceWavPubGUID {
				t.Fatalf("Resolve() = %+v, want source wavpub_guid", decision)
			}
		})
	}
}

func TestResolveWavPubFeedIdentityIgnoresSchemeCaseAndTrailingDot(t *testing.T) {
	for _, feedURL := range []string{
		"https://hosting.wavpub.cn/pie/feed/",
		"http://hosting.wavpub.cn/pie/feed/",
		"https://HOSTING.WAVPUB.CN/pie/feed/",
		"https://hosting.wavpub.cn.:8443/pie/feed/",
	} {
		decision := Resolve(Input{
			Feed: FeedIdentity{FeedURL: feedURL},
			GUID: "https://hosting.wavpub.cn/pie/ep229/",
		})
		if decision.Source != SourceWavPubGUID {
			t.Fatalf("Resolve(feed %q) = %+v, want wavpub_guid", feedURL, decision)
		}
	}
}

func TestResolveWavPubProxyFeedWithWordPressPageGUID(t *testing.T) {
	decision := Resolve(Input{
		Feed: FeedIdentity{FeedURL: "https://proxy.wavpub.com/pie.xml"},
		GUID: "https://hosting.wavpub.cn/pie/?p=822",
	})
	if decision.Source != SourceWavPubGUID || decision.URL != "https://hosting.wavpub.cn/pie/?p=822" {
		t.Fatalf("Resolve() = %+v, want the verified WavPub proxy GUID", decision)
	}
}

func TestResolveWavPubProxyFeedRequiresExactVerifiedEndpoint(t *testing.T) {
	for _, feedURL := range []string{
		"http://proxy.wavpub.com/pie.xml",
		"https://proxy.wavpub.com/pie.xml/",
		"https://proxy.wavpub.com/pie.xml?source=other",
		"https://proxy.wavpub.com/other.xml",
		"https://proxy.wavpub.com:8443/pie.xml",
		"https://proxy.wavpub.com.evil.example/pie.xml",
	} {
		decision := Resolve(Input{
			Feed: FeedIdentity{FeedURL: feedURL},
			GUID: "https://hosting.wavpub.cn/pie/?p=822",
		})
		if decision.Source != SourceNone || decision.URL != "" {
			t.Fatalf("Resolve(feed %q) = %+v, want no proxy GUID fallback", feedURL, decision)
		}
	}
}

func TestResolveUnusableRSSLinkDoesNotClearExistingLink(t *testing.T) {
	for _, link := range []string{
		"javascript:alert(1)",
		"data:text/html,<p>x</p>",
		"https://",
		"http://:80/episode/1",
		"https:///path",
		"/episode/1",
		"not a url",
	} {
		decision := Resolve(Input{
			RSSLink:      link,
			ExistingLink: "https://example.com/episode/1",
		})
		if decision.Source != SourceExisting || decision.URL != "https://example.com/episode/1" {
			t.Fatalf("Resolve(link %q) = %+v, want the existing link kept", link, decision)
		}
	}
}

func TestResolveWavPubFallbackWinsOverExistingLink(t *testing.T) {
	decision := Resolve(Input{
		Feed:         FeedIdentity{FeedURL: "https://hosting.wavpub.cn/pie/feed/"},
		GUID:         "https://hosting.wavpub.cn/pie/ep229/",
		ExistingLink: "https://stale.example.com/episode/229",
	})
	if decision.Source != SourceWavPubGUID || decision.URL != "https://hosting.wavpub.cn/pie/ep229/" {
		t.Fatalf("Resolve() = %+v, want the WavPub page GUID", decision)
	}
}
