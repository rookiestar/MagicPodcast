package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const validatorUserAgent = "MagicPodcast-PodcastIndex-Validator/1"

// NewHTTPClient creates a client with direct access by default. A proxy is
// used only when the caller explicitly supplies an HTTP(S) proxy URL.
func NewHTTPClient(timeout time.Duration, proxyURL string) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if strings.TrimSpace(proxyURL) != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy URL: %w", err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("unsupported proxy scheme %q; use http or https", parsed.Scheme)
		}
		if parsed.Host == "" {
			return nil, fmt.Errorf("proxy URL must include a host")
		}
		if parsed.User != nil {
			return nil, fmt.Errorf("proxy URL must not include credentials")
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	transport.MaxIdleConns = 4
	transport.MaxIdleConnsPerHost = 2
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}, nil
}

func NewDirectHTTPClient(timeout time.Duration) *http.Client {
	client, err := NewHTTPClient(timeout, "")
	if err != nil {
		// An empty proxy URL is validated by construction and cannot fail. Keep
		// the historical no-error API for callers that require direct access.
		panic(err)
	}
	return client
}

// ProxyEndpoint returns a credential-free endpoint suitable for an audit
// manifest. It intentionally omits user information and query/fragment data.
func ProxyEndpoint(proxyURL string) (string, error) {
	if strings.TrimSpace(proxyURL) == "" {
		return "", nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return "", fmt.Errorf("parse proxy URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported proxy scheme %q; use http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("proxy URL must include a host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("proxy URL must not include credentials")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func FingerprintURL(ctx context.Context, client *http.Client, sourceURL string, now func() time.Time) (Fingerprint, error) {
	if strings.TrimSpace(sourceURL) == "" {
		return Fingerprint{}, fmt.Errorf("source URL is empty")
	}
	if client == nil {
		client = NewDirectHTTPClient(30 * time.Second)
	}
	if now == nil {
		now = time.Now
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, sourceURL, nil)
	if err != nil {
		return Fingerprint{URL: sourceURL, CheckedAt: now()}, fmt.Errorf("create HEAD request: %w", err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", validatorUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return Fingerprint{URL: sourceURL, CheckedAt: now()}, fmt.Errorf("HEAD %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()

	fingerprint := fingerprintFromResponse(resp, now())
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fingerprint, fmt.Errorf("HEAD %s returned HTTP %d", sourceURL, resp.StatusCode)
	}
	if err := ValidateFingerprint(fingerprint); err != nil {
		return fingerprint, err
	}
	return fingerprint, nil
}

func fingerprintFromResponse(resp *http.Response, checkedAt time.Time) Fingerprint {
	contentLength := resp.ContentLength
	if contentLength < 0 {
		contentLength = 0
	}
	responseURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		responseURL = resp.Request.URL.String()
	}
	return Fingerprint{
		URL:                responseURL,
		StatusCode:         resp.StatusCode,
		ContentLength:      contentLength,
		ContentType:        strings.TrimSpace(resp.Header.Get("Content-Type")),
		ETag:               strings.TrimSpace(resp.Header.Get("ETag")),
		LastModified:       strings.TrimSpace(resp.Header.Get("Last-Modified")),
		AcceptRanges:       strings.TrimSpace(resp.Header.Get("Accept-Ranges")),
		ContentDisposition: strings.TrimSpace(resp.Header.Get("Content-Disposition")),
		CheckedAt:          checkedAt,
		Headers: map[string]string{
			"content-length":      strings.TrimSpace(resp.Header.Get("Content-Length")),
			"content-type":        strings.TrimSpace(resp.Header.Get("Content-Type")),
			"etag":                strings.TrimSpace(resp.Header.Get("ETag")),
			"last-modified":       strings.TrimSpace(resp.Header.Get("Last-Modified")),
			"accept-ranges":       strings.TrimSpace(resp.Header.Get("Accept-Ranges")),
			"content-disposition": strings.TrimSpace(resp.Header.Get("Content-Disposition")),
		},
	}
}

func ValidateFingerprint(fingerprint Fingerprint) error {
	if fingerprint.StatusCode < http.StatusOK || fingerprint.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("source returned HTTP %d", fingerprint.StatusCode)
	}
	if fingerprint.ContentLength <= 0 {
		return fmt.Errorf("source did not provide a positive Content-Length")
	}
	if fingerprint.ContentType == "" {
		return fmt.Errorf("source did not provide Content-Type")
	}
	if fingerprint.ETag == "" && fingerprint.LastModified == "" {
		return fmt.Errorf("source did not provide ETag or Last-Modified for object identity")
	}
	if !isArchiveContentType(fingerprint.ContentType) {
		return fmt.Errorf("unexpected archive Content-Type %q", fingerprint.ContentType)
	}
	return nil
}

func isArchiveContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch mediaType {
	case "application/gzip", "application/x-gzip", "application/octet-stream", "binary/octet-stream", "application/x-tar":
		return true
	default:
		return false
	}
}

func CompareFingerprints(before, after Fingerprint) error {
	var differences []string
	if before.URL != after.URL {
		differences = append(differences, "url")
	}
	if before.StatusCode != after.StatusCode {
		differences = append(differences, "status_code")
	}
	if before.ContentLength != after.ContentLength {
		differences = append(differences, "content_length")
	}
	if before.ContentType != after.ContentType {
		differences = append(differences, "content_type")
	}
	if before.ETag != after.ETag {
		differences = append(differences, "etag")
	}
	if before.LastModified != after.LastModified {
		differences = append(differences, "last_modified")
	}
	if len(differences) > 0 {
		return fmt.Errorf("source object metadata changed during download: %s", strings.Join(differences, ", "))
	}
	return nil
}

func SHA256File(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	bytesCopied, err := io.Copy(hash, file)
	if err != nil {
		return "", bytesCopied, fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), bytesCopied, nil
}

// VerifyFileIdentity recomputes a local file's size and SHA-256 and compares
// them with the identity recorded before staging. An empty expected value is
// treated as unavailable evidence rather than as a wildcard by the caller;
// this helper only skips that individual comparison so callers can report the
// missing-evidence blocker together with other validation results.
func VerifyFileIdentity(path, expectedSHA256 string, expectedSize int64) (string, int64, error) {
	actualSHA256, actualSize, err := SHA256File(path)
	if err != nil {
		return "", actualSize, err
	}
	if expectedSize > 0 && actualSize != expectedSize {
		return actualSHA256, actualSize, fmt.Errorf("file size changed: got=%d expected=%d", actualSize, expectedSize)
	}
	if expectedSHA256 != "" && !strings.EqualFold(actualSHA256, expectedSHA256) {
		return actualSHA256, actualSize, fmt.Errorf("file SHA-256 changed: got=%s expected=%s", actualSHA256, expectedSHA256)
	}
	return actualSHA256, actualSize, nil
}

func EnsureStagingSeparate(stagingDir, liveDB string) error {
	if strings.TrimSpace(liveDB) == "" {
		return nil
	}
	stageAbs, err := filepath.Abs(stagingDir)
	if err != nil {
		return fmt.Errorf("resolve staging path: %w", err)
	}
	liveAbs, err := filepath.Abs(liveDB)
	if err != nil {
		return fmt.Errorf("resolve live database path: %w", err)
	}
	liveDir := filepath.Dir(liveAbs)
	stageSafe, err := resolveExistingPath(stageAbs)
	if err != nil {
		return err
	}
	liveDirSafe, err := resolveExistingPath(liveDir)
	if err != nil {
		return err
	}
	if sameOrWithin(stageSafe, liveDirSafe) || sameOrWithin(liveDirSafe, stageSafe) {
		return fmt.Errorf("staging directory %s must be independent from live database directory %s", stageAbs, liveDir)
	}
	return nil
}

func resolveExistingPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %s: %w", path, err)
	}
	missing := make([]string, 0)
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve symlinks for %s: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path), nil
		}
		missing = append(missing, filepath.Base(current))
	}
}

func sameOrWithin(path, parent string) bool {
	path = filepath.Clean(path)
	parent = filepath.Clean(parent)
	if path == parent {
		return true
	}
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
