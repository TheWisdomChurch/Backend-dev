package service

import (
	"image"
	"testing"
)

func TestApplyTargetAspectRatioNoOpWhenTargetNotPositive(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1000, 400))

	for _, target := range []float64{0, -1, -0.5} {
		out, cropped := applyTargetAspectRatio(src, target)
		if cropped {
			t.Fatalf("target=%v: expected no crop, got cropped=true", target)
		}
		if out.Bounds() != src.Bounds() {
			t.Fatalf("target=%v: expected unchanged bounds, got %v", target, out.Bounds())
		}
	}
}

func TestApplyTargetAspectRatioNoOpWhenAlreadyMatching(t *testing.T) {
	// 1600x900 is exactly 16:9.
	src := image.NewNRGBA(image.Rect(0, 0, 1600, 900))

	out, cropped := applyTargetAspectRatio(src, AllowedAspectRatios["16:9"])
	if cropped {
		t.Fatalf("expected no crop for an already-matching source, got cropped=true")
	}
	if out.Bounds().Dx() != 1600 || out.Bounds().Dy() != 900 {
		t.Fatalf("expected unchanged 1600x900, got %dx%d", out.Bounds().Dx(), out.Bounds().Dy())
	}
}

func TestApplyTargetAspectRatioNoOpWithinTolerance(t *testing.T) {
	// 1601x900 is a hair off exact 16:9, well inside the 0.5% tolerance.
	src := image.NewNRGBA(image.Rect(0, 0, 1601, 900))

	out, cropped := applyTargetAspectRatio(src, AllowedAspectRatios["16:9"])
	if cropped {
		t.Fatalf("expected no crop for a within-tolerance source, got cropped=true")
	}
	if out.Bounds().Dx() != 1601 {
		t.Fatalf("expected unchanged width 1601, got %d", out.Bounds().Dx())
	}
}

func TestApplyTargetAspectRatioCropsWideSourceOnWidth(t *testing.T) {
	// 2000x900 is wider than 16:9 (2.22:1 vs 1.78:1) — width should crop,
	// height must stay untouched (and therefore never upscale).
	src := image.NewNRGBA(image.Rect(0, 0, 2000, 900))

	out, cropped := applyTargetAspectRatio(src, AllowedAspectRatios["16:9"])
	if !cropped {
		t.Fatalf("expected a crop for a too-wide source")
	}
	if out.Bounds().Dy() != 900 {
		t.Fatalf("expected height to stay 900, got %d", out.Bounds().Dy())
	}
	wantWidth := int(900.0 * AllowedAspectRatios["16:9"])
	if got := out.Bounds().Dx(); abs(got-wantWidth) > 1 {
		t.Fatalf("expected width ~%d, got %d", wantWidth, got)
	}
}

func TestApplyTargetAspectRatioCropsTallSourceOnHeight(t *testing.T) {
	// 1000x1000 (1:1) is taller than 16:9 — height should crop, width
	// must stay untouched.
	src := image.NewNRGBA(image.Rect(0, 0, 1000, 1000))

	out, cropped := applyTargetAspectRatio(src, AllowedAspectRatios["16:9"])
	if !cropped {
		t.Fatalf("expected a crop for a too-tall source")
	}
	if out.Bounds().Dx() != 1000 {
		t.Fatalf("expected width to stay 1000, got %d", out.Bounds().Dx())
	}
	wantHeight := int(1000.0 / AllowedAspectRatios["16:9"])
	if got := out.Bounds().Dy(); abs(got-wantHeight) > 1 {
		t.Fatalf("expected height ~%d, got %d", wantHeight, got)
	}
}

func TestApplyTargetAspectRatioNeverUpscales(t *testing.T) {
	// A small source cropped to a new ratio should only ever shrink one
	// dimension, never grow either one beyond the original.
	src := image.NewNRGBA(image.Rect(0, 0, 500, 500))

	out, cropped := applyTargetAspectRatio(src, AllowedAspectRatios["4:5"])
	if !cropped {
		t.Fatalf("expected a crop for a 1:1 source against a 4:5 target")
	}
	if out.Bounds().Dx() > 500 || out.Bounds().Dy() > 500 {
		t.Fatalf("expected no upscaling, got %dx%d from a 500x500 source", out.Bounds().Dx(), out.Bounds().Dy())
	}
}

func TestAllowedAspectRatiosUnknownKeyIsNoEnforcement(t *testing.T) {
	// This mirrors how upload_handler.go resolves the incoming form value:
	// map lookup on a missing/unknown/empty key yields the zero value,
	// which applyTargetAspectRatio treats as "no enforcement."
	for _, key := range []string{"", "3:2", "not-a-ratio", "16:9 "} {
		if ratio, ok := AllowedAspectRatios[key]; ok {
			t.Fatalf("key %q unexpectedly present in allow-list with ratio %v", key, ratio)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
