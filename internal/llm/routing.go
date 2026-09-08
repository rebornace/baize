package llm

import "strings"

// AutoProfileID is the sentinel value for the "smart routing (Auto)" choice.
// It is NOT a real model profile id: a turn tagged with it lets the router pick
// a concrete model based on the turn's content (e.g. routing image turns to a
// vision-capable profile). The interactive chat exposes it as an explicit
// "智能路由 / Auto" option; channel inbound (WeChat etc.) has no manual picker
// and is always Auto.
const AutoProfileID = "auto"

// RoutingProfile is the subset of a stored model profile the router needs. It
// is defined here (rather than taking store.ModelProfile) so the llm package
// does not depend on the store package.
type RoutingProfile struct {
	ID             string
	SupportsVision bool
	IsDefault      bool
}

// ProfileSelection is the outcome of resolving which model a turn uses.
type ProfileSelection struct {
	// ProfileID is the model profile to pin the run to. "" means "use the
	// server default" (no override).
	ProfileID string
	// Auto reports whether the choice was made by the auto router (vs. an
	// explicit manual profile selection).
	Auto bool
}

// NormalizeProfileChoice maps a raw per-run model choice to either the Auto
// sentinel or a concrete profile id. An empty/unset choice means Auto (the
// interactive default and the only mode for channel inbound).
func NormalizeProfileChoice(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, AutoProfileID) {
		return AutoProfileID
	}
	return raw
}

// ResolveProfileForImages decides which model serves a turn.
//
// choice is the normalized per-run choice (AutoProfileID or a concrete id).
// hasImages reports whether the turn actually carries image content (drives
// routing — a docx/pdf that only extracts text is not an image turn).
// defaultVision reports whether the active default provider supports vision.
// profiles is the current set of configured model profiles.
//
// Semantics:
//   - No images: the choice is honored directly (Auto -> default ""; manual ->
//     that profile id). visionOK is always true.
//   - Images + Auto: when the default already supports vision there is no
//     override (""); otherwise a vision-capable profile is auto-selected. With
//     no vision model available, ProfileID is "" and visionOK is false (the
//     caller degrades to a text note for channels, or warns for the web).
//   - Images + manual profile: the chosen id is ALWAYS honored — the router
//     never silently reroutes a manual choice. visionOK reflects whether that
//     specific profile supports vision; when false the caller warns the user
//     that their selected model cannot see the image.
func ResolveProfileForImages(choice string, hasImages, defaultVision bool, profiles []RoutingProfile) (sel ProfileSelection, visionOK bool) {
	auto := choice == "" || choice == AutoProfileID

	if !hasImages {
		if auto {
			return ProfileSelection{ProfileID: "", Auto: true}, true
		}
		return ProfileSelection{ProfileID: choice, Auto: false}, true
	}

	if auto {
		if defaultVision {
			return ProfileSelection{ProfileID: "", Auto: true}, true
		}
		if id := PickVisionProfile(profiles); id != "" {
			return ProfileSelection{ProfileID: id, Auto: true}, true
		}
		// Auto but no vision model exists: stay on the default; the caller
		// degrades/warns because the images cannot be seen.
		return ProfileSelection{ProfileID: "", Auto: true}, false
	}

	// Manual selection: honor it exactly, and report whether it can see images.
	if profileSupportsVision(choice, profiles) {
		return ProfileSelection{ProfileID: choice, Auto: false}, true
	}
	return ProfileSelection{ProfileID: choice, Auto: false}, false
}

// PickVisionProfile returns a vision-capable profile id, preferring a
// non-default one (a vision-capable default is already served directly via
// defaultVision), then falling back to any vision-capable profile. Returns ""
// when no vision profile exists.
func PickVisionProfile(profiles []RoutingProfile) string {
	for _, p := range profiles {
		if p.SupportsVision && !p.IsDefault {
			return p.ID
		}
	}
	for _, p := range profiles {
		if p.SupportsVision {
			return p.ID
		}
	}
	return ""
}

func profileSupportsVision(id string, profiles []RoutingProfile) bool {
	for _, p := range profiles {
		if p.ID == id {
			return p.SupportsVision
		}
	}
	return false
}
