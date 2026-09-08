package llm

import "testing"

func TestNormalizeProfileChoice(t *testing.T) {
	cases := map[string]string{
		"":        AutoProfileID,
		"  ":      AutoProfileID,
		"auto":    AutoProfileID,
		" AUTO ":  AutoProfileID,
		"mp_123":  "mp_123",
		" mp_123": "mp_123",
	}
	for in, want := range cases {
		if got := NormalizeProfileChoice(in); got != want {
			t.Errorf("NormalizeProfileChoice(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickVisionProfile(t *testing.T) {
	// No profiles.
	if got := PickVisionProfile(nil); got != "" {
		t.Fatalf("empty: got %q want empty", got)
	}
	// Text-only default + a vision non-default => the vision one.
	got := PickVisionProfile([]RoutingProfile{
		{ID: "mp_default", SupportsVision: false, IsDefault: true},
		{ID: "mp_vision", SupportsVision: true, IsDefault: false},
	})
	if got != "mp_vision" {
		t.Fatalf("got %q want mp_vision", got)
	}
	// Only a vision-capable default => still returns it (fallback).
	got = PickVisionProfile([]RoutingProfile{
		{ID: "mp_default", SupportsVision: true, IsDefault: true},
	})
	if got != "mp_default" {
		t.Fatalf("got %q want mp_default", got)
	}
	// Non-vision profiles only => "".
	got = PickVisionProfile([]RoutingProfile{
		{ID: "mp_a", SupportsVision: false, IsDefault: true},
		{ID: "mp_b", SupportsVision: false},
	})
	if got != "" {
		t.Fatalf("got %q want empty (no vision)", got)
	}
}

func TestResolveProfileForImages(t *testing.T) {
	visionProfiles := []RoutingProfile{
		{ID: "mp_default", SupportsVision: false, IsDefault: true},
		{ID: "mp_vision", SupportsVision: true},
	}

	t.Run("auto text-only turn uses default", func(t *testing.T) {
		sel, ok := ResolveProfileForImages(AutoProfileID, false, false, visionProfiles)
		if !ok || sel.ProfileID != "" || !sel.Auto {
			t.Fatalf("got %+v ok=%v, want auto->default", sel, ok)
		}
	})

	t.Run("manual text-only turn honors choice", func(t *testing.T) {
		sel, ok := ResolveProfileForImages("mp_default", false, false, visionProfiles)
		if !ok || sel.ProfileID != "mp_default" || sel.Auto {
			t.Fatalf("got %+v ok=%v, want manual mp_default", sel, ok)
		}
	})

	t.Run("auto image + vision model routes to vision", func(t *testing.T) {
		sel, ok := ResolveProfileForImages(AutoProfileID, true, false, visionProfiles)
		if !ok || sel.ProfileID != "mp_vision" || !sel.Auto {
			t.Fatalf("got %+v ok=%v, want auto->mp_vision", sel, ok)
		}
	})

	t.Run("auto image + default already vision stays default", func(t *testing.T) {
		sel, ok := ResolveProfileForImages(AutoProfileID, true, true, visionProfiles)
		if !ok || sel.ProfileID != "" || !sel.Auto {
			t.Fatalf("got %+v ok=%v, want auto->default (no override)", sel, ok)
		}
	})

	t.Run("auto image + no vision model => not ok", func(t *testing.T) {
		textOnly := []RoutingProfile{{ID: "mp_default", SupportsVision: false, IsDefault: true}}
		sel, ok := ResolveProfileForImages(AutoProfileID, true, false, textOnly)
		if ok || sel.ProfileID != "" || !sel.Auto {
			t.Fatalf("got %+v ok=%v, want auto->default with ok=false", sel, ok)
		}
	})

	t.Run("manual vision model + image => honored, ok", func(t *testing.T) {
		sel, ok := ResolveProfileForImages("mp_vision", true, false, visionProfiles)
		if !ok || sel.ProfileID != "mp_vision" || sel.Auto {
			t.Fatalf("got %+v ok=%v, want manual mp_vision", sel, ok)
		}
	})

	t.Run("manual text-only model + image => honored but NOT ok (no reroute)", func(t *testing.T) {
		sel, ok := ResolveProfileForImages("mp_default", true, false, visionProfiles)
		// mp_default exists and is text-only; the router must NOT reroute to
		// mp_vision; visionOK=false so the caller warns the user.
		if ok || sel.ProfileID != "mp_default" || sel.Auto {
			t.Fatalf("got %+v ok=%v, want manual mp_default with ok=false (no silent reroute)", sel, ok)
		}
	})

	t.Run("manual unknown model id + image => treated as not vision", func(t *testing.T) {
		sel, ok := ResolveProfileForImages("mp_gone", true, false, visionProfiles)
		if ok || sel.ProfileID != "mp_gone" {
			t.Fatalf("got %+v ok=%v, want mp_gone with ok=false", sel, ok)
		}
	})
}
