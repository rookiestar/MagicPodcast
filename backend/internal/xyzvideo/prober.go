package xyzvideo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"magicpodcast/internal/models"
)

const (
	maxPlaybackBodyBytes = 1 << 20
	playbackTimeout      = 15 * time.Second
	defaultUserAgent     = "MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)"
)

// Getter is the testable HTTP seam. Implementations must not persist the body.
type Getter interface {
	Get(ctx context.Context, rawURL string) (status int, body []byte, err error)
}

// Slotter serializes probes onto the shared Xiaoyuzhou Feed queue without
// treating the JSON as a Feed snapshot.
type Slotter interface {
	AcquireDomainSlot(ctx context.Context, rawURL string) (func(), error)
	ObserveDomainProbe(rawURL string, status int, err error)
}

// Outcome is the probe result written to the tri-state column. HaltBatch asks
// the caller to stop remaining probes for this podcast after a WAF or outage.
type Outcome struct {
	Availability string
	HaltBatch    bool
}

// ProberConfig wires the HTTP getter, optional shared-queue slotter, and the
// playback API origin. Tests point BaseURL at httptest.
type ProberConfig struct {
	Getter    Getter
	Slotter   Slotter
	BaseURL   string
	UserAgent string
}

// Prober reads Xiaoyuzhou's public video-playback endpoint and returns only a
// tri-state. It never returns or retains signed HLS URLs.
type Prober struct {
	getter  Getter
	slotter Slotter
	baseURL string
}

func NewProber(config ProberConfig) *Prober {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBase
	}
	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	getter := config.Getter
	if getter == nil {
		getter = NewHTTPGetter(nil, userAgent)
	}
	return &Prober{
		getter:  getter,
		slotter: config.Slotter,
		baseURL: baseURL,
	}
}

func (p *Prober) Probe(ctx context.Context, episodeID string) Outcome {
	if p == nil || p.getter == nil || !episodeIDPattern.MatchString(episodeID) {
		return Outcome{Availability: models.VideoAvailabilityUnknown}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rawURL := p.baseURL + "/api/episodes/" + url.PathEscape(episodeID) + "/video-playback"
	if p.slotter != nil {
		release, err := p.slotter.AcquireDomainSlot(ctx, rawURL)
		if err != nil {
			return Outcome{Availability: models.VideoAvailabilityUnknown, HaltBatch: true}
		}
		if release != nil {
			defer release()
		}
	}

	status, body, err := p.getter.Get(ctx, rawURL)
	if p.slotter != nil {
		p.slotter.ObserveDomainProbe(rawURL, status, err)
	}
	if err != nil {
		return Outcome{Availability: models.VideoAvailabilityUnknown, HaltBatch: true}
	}
	// Drop the body immediately after classification so auth_key / m3u8 cannot
	// leak into callers or logs.
	availability := ParsePlaybackResponse(status, body)
	return Outcome{
		Availability: availability,
		HaltBatch:    haltAfterStatus(status, err),
	}
}

// HTTPGetter performs a cookie-less GET with a 1 MiB body cap.
type HTTPGetter struct {
	client    *http.Client
	userAgent string
}

func NewHTTPGetter(client *http.Client, userAgent string) *HTTPGetter {
	if client == nil {
		client = &http.Client{
			Timeout: playbackTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("stopped after 5 redirects")
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return fmt.Errorf("redirect scheme rejected")
				}
				return nil
			},
		}
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = defaultUserAgent
	}
	return &HTTPGetter{client: client, userAgent: userAgent}
}

func (g *HTTPGetter) Get(ctx context.Context, rawURL string) (int, []byte, error) {
	if g == nil || g.client == nil {
		return 0, nil, fmt.Errorf("video probe getter is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPlaybackBodyBytes+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(body) > maxPlaybackBodyBytes {
		return resp.StatusCode, nil, fmt.Errorf("video playback body exceeded %d bytes", maxPlaybackBodyBytes)
	}
	return resp.StatusCode, body, nil
}
