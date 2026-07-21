package feed

import (
	"crypto/tls"
	"net/http/httptrace"

	"github.com/sirupsen/logrus"
)

// newFeedFetchTrace returns an httptrace.ClientTrace that advances the shared
// failure-phase pointer as the request progresses: dns -> connect -> tls ->
// response_header. The body_read phase is set by the Fetcher once it begins
// reading/parsing the response. The phase is only stamped onto the outcome and
// the structured failure log when the request fails, so a status-level refusal
// (e.g. 403) — which always follows GotFirstResponseByte — is reported as
// response_header/body_read, never connect.
func newFeedFetchTrace(phase *FailurePhase) *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			*phase = FailurePhaseDNS
		},
		ConnectStart: func(string, string) {
			*phase = FailurePhaseConnect
		},
		ConnectDone: func(string, string, error) {
			*phase = FailurePhaseConnect
		},
		TLSHandshakeStart: func() {
			*phase = FailurePhaseTLS
		},
		// On a TLS failure phase stays tls (set by TLSHandshakeStart). On
		// success the Fetcher stamps response_header once headers arrive.
		TLSHandshakeDone: func(tls.ConnectionState, error) {},
		GotFirstResponseByte: func() {
			*phase = FailurePhaseResponseHeader
		},
	}
}

// feedFailureLogFields returns the bounded, whitelisted structured-log fields
// for a Feed fetch failure. It deliberately contains no response body, cookies,
// credentials, or arbitrary response headers — only the diagnostic metadata
// needed to classify and aggregate the failure.
func feedFailureLogFields(result *FetchResult, safeURL string) logrus.Fields {
	fields := logrus.Fields{
		"feed_url":                safeURL,
		"target_domain":           result.Access.TargetDomain,
		"error_category":          string(result.Access.ErrorCategory),
		"failure_phase":           string(result.Access.FailurePhase),
		"configured_egress_label": result.Access.EgressID,
		"circuit_state":           string(result.Access.CircuitState),
		"response_time_ms":        result.Access.ResponseTimeMs,
		"response_bytes":          result.Access.ResponseBytes,
		"cache_status":            string(result.Access.CacheStatus),
		"freshness":               string(result.Access.Freshness),
	}
	if result.Access.HTTPStatus != nil {
		fields["http_status"] = *result.Access.HTTPStatus
	}
	if result.Access.RetryAfter != "" {
		fields["retry_after"] = result.Access.RetryAfter
	}
	if result.Access.RetrievedAt != nil {
		fields["snapshot_retrieved_at"] = *result.Access.RetrievedAt
	}
	return fields
}
