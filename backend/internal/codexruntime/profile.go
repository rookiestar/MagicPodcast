package codexruntime

import "sort"

// ModelProfileID is the stable profile identifier exchanged with callers.
// Pages and upper modules may only reference these IDs; the trusted model,
// reasoning effort, and speed configuration is resolved inside this package
// and is never reconstructed or widened elsewhere.
type ModelProfileID string

// DefaultModelProfileID is the product default applied when a caller does not
// state a profile ID.
const DefaultModelProfileID = ModelProfileID("balanced")

// ModelProfile is the trusted technical configuration a stable profile ID
// resolves to. An empty ServiceTier means the standard tier: the Fast tier is
// only ever expressed by explicitly requesting it, never by downgrading.
// ServiceTier is the account catalog's wire ID for the tier (currently
// "priority"); ServiceTierName is its stable human name ("Fast").
type ModelProfile struct {
	ID              ModelProfileID
	Model           string
	Effort          string
	ServiceTier     string
	ServiceTierName string
}

// modelProfiles is the single technical profile catalog. Entries are fixed at
// build time so a page can never inject arbitrary models, reasoning efforts,
// or service tiers. The verified 0.147.0 account catalog exposes the Fast
// speed tier under the wire ID "priority" with the display name "Fast"
// ("1.5x speed, increased usage"); Standard is the absent tier.
var modelProfiles = map[ModelProfileID]ModelProfile{
	ModelProfileID("quick"): {
		ID:              ModelProfileID("quick"),
		Model:           "gpt-5.6-sol",
		Effort:          "medium",
		ServiceTier:     "priority",
		ServiceTierName: "Fast",
	},
	DefaultModelProfileID: {
		ID:              DefaultModelProfileID,
		Model:           "gpt-5.6-luna",
		Effort:          "max",
		ServiceTier:     "priority",
		ServiceTierName: "Fast",
	},
	ModelProfileID("deep"): {
		ID:              ModelProfileID("deep"),
		Model:           "gpt-5.6-sol",
		Effort:          "xhigh",
		ServiceTier:     "",
		ServiceTierName: "Standard",
	},
}

// ResolveModelProfile resolves a stable profile ID to its trusted
// configuration. Unknown IDs return false and must be rejected by callers
// before reaching the runtime; no fallback profile is ever substituted.
func ResolveModelProfile(id ModelProfileID) (ModelProfile, bool) {
	profile, exists := modelProfiles[id]
	return profile, exists
}

// ModelProfiles returns every catalog entry in stable ID order so callers can
// expose the technical meaning of each profile without copying the mapping.
func ModelProfiles() []ModelProfile {
	ids := make([]string, 0, len(modelProfiles))
	for id := range modelProfiles {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	profiles := make([]ModelProfile, 0, len(ids))
	for _, id := range ids {
		profiles = append(profiles, modelProfiles[ModelProfileID(id)])
	}
	return profiles
}
