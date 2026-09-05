// Package originallink is the single resolution entry for an episode's
// original web page link (原节目链接). RSS sync and maintenance commands use
// Resolve instead of independently interpreting RSS link fields and GUIDs.
package originallink

import (
	"fmt"
	"net/url"
	"strings"
)

// Source names where a resolved link came from. Values are stable audit
// tokens, not persisted database fields.
type Source string

const (
	// SourceNone means no usable candidate was found; the stored link stays empty.
	SourceNone Source = "none"
	// SourceRSSLink is a usable standard RSS <link>.
	SourceRSSLink Source = "rss_link"
	// SourceWavPubGUID is a strictly matched WavPub page GUID fallback.
	SourceWavPubGUID Source = "wavpub_guid"
	// SourceExisting is the retained non-empty link already in the database.
	SourceExisting Source = "existing"
)

// WavPubHost is the exact GUID host the WavPub fallback accepts. It is the
// only podcast-hosting domain with a first-hand verified mapping between
// page GUIDs and real episode pages.
const WavPubHost = "hosting.wavpub.cn"

const (
	wavPubProxyHost     = "proxy.wavpub.com"
	wavPubProxyFeedPath = "/pie.xml"
)

// FeedIdentity identifies the feed the RSS item belongs to. The subscription
// feed URL is the evidence for source-specific rules, so feed content cannot
// borrow another host's fallback by declaring it inside the feed itself.
type FeedIdentity struct {
	FeedURL string
}

// Input carries everything the resolution may look at. RSSLink is the
// standard RSS <item><link>, GUID the raw RSS item GUID, and ExistingLink
// the non-empty-or-empty value already stored for this episode.
type Input struct {
	Feed         FeedIdentity
	RSSLink      string
	GUID         string
	ExistingLink string
}

// Decision reports the chosen URL (empty means 暂缺), where it came from,
// and a human-readable reason for audits and troubleshooting.
type Decision struct {
	URL    string
	Source Source
	Reason string
}

// Resolve picks the original link for one RSS item. Priority: usable standard
// <link>; a strictly matched source rule; the existing non-empty link; none.
// An empty or unusable feed value never clears an existing usable link.
func Resolve(in Input) Decision {
	if link := strings.TrimSpace(in.RSSLink); usableWebURL(link) {
		return Decision{URL: link, Source: SourceRSSLink, Reason: "标准 RSS link 可用，优先采用"}
	}

	if isWavPubFeed(in.Feed) {
		if guid := strings.TrimSpace(in.GUID); isWavPubPageGUID(guid) {
			return Decision{
				URL:    guid,
				Source: SourceWavPubGUID,
				Reason: "标准 link 缺失，命中经验证的 WavPub 页面 GUID 回退",
			}
		}
	}

	if existing := strings.TrimSpace(in.ExistingLink); existing != "" {
		return Decision{URL: existing, Source: SourceExisting, Reason: "Feed 没有可用链接，保留数据库已有非空链接"}
	}

	return Decision{URL: "", Source: SourceNone, Reason: noCandidateReason(in)}
}

func noCandidateReason(in Input) string {
	if strings.TrimSpace(in.RSSLink) != "" {
		return "标准 RSS link 不是合法的绝对 HTTP/HTTPS 地址"
	}
	if isWavPubFeed(in.Feed) && strings.TrimSpace(in.GUID) != "" {
		return "WavPub GUID 未通过安全规则：必须是 hosting.wavpub.cn 的 HTTPS 非根页面"
	}
	if strings.TrimSpace(in.GUID) != "" {
		return "Feed 未命中经验证的 GUID 页面规则"
	}
	return "没有可用的原节目链接候选"
}

// usableWebURL accepts only an absolute http(s) URL with a host. Scheme-only
// strings, missing hosts, relative paths, parse failures, and dangerous
// protocols are all rejected.
func usableWebURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Hostname() != ""
}

// isWavPubFeed reports whether the subscription feed is one of the verified
// WavPub endpoints. The proxy endpoint is intentionally matched by its exact
// HTTPS path because it serves more than one possible feed identity.
func isWavPubFeed(feed FeedIdentity) bool {
	if exactHost(feed.FeedURL) == WavPubHost {
		return true
	}

	parsed, err := url.Parse(strings.TrimSpace(feed.FeedURL))
	if err != nil || parsed.User != nil || parsed.Port() != "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") &&
		normalizeHost(parsed.Hostname()) == wavPubProxyHost &&
		parsed.EscapedPath() == wavPubProxyFeedPath &&
		parsed.RawQuery == "" &&
		!parsed.ForceQuery &&
		parsed.Fragment == ""
}

// isWavPubGUID accepts only an absolute HTTPS URL on the exact WavPub host
// that points at a real page instead of the site root.
func isWavPubPageGUID(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" {
		return false
	}
	if normalizeHost(parsed.Hostname()) != WavPubHost {
		return false
	}
	return len(pathSegments(parsed.Path)) > 0
}

func exactHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return normalizeHost(parsed.Hostname())
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func pathSegments(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
}

// String renders the decision for logs and audit lines.
func (d Decision) String() string {
	return fmt.Sprintf("source=%s url=%q reason=%s", d.Source, d.URL, d.Reason)
}
