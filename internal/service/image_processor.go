package service

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/disintegration/imaging"
)

// ImageVariantName identifies one derived size in the fixed variant set every
// processed image gets. Callers pick the right one for context (a list view
// wants Thumbnail, a detail page wants Large) instead of always shipping the
// original — the same responsive-image pattern used by every major photo
// product (Instagram, Facebook, etc.), just with a small fixed size ladder
// instead of a generated srcset.
type ImageVariantName string

const (
	ImageVariantThumbnail ImageVariantName = "thumbnail"
	ImageVariantMedium    ImageVariantName = "medium"
	ImageVariantLarge     ImageVariantName = "large"
)

// imageVariantWidths defines the size ladder, largest first so ProcessImage
// can reuse each downscale step as the source for the next (cheaper than
// resizing from the original every time, and produces more consistent
// results than three independent resizes).
var imageVariantWidths = []struct {
	name  ImageVariantName
	width int
}{
	{ImageVariantLarge, 1600},
	{ImageVariantMedium, 800},
	{ImageVariantThumbnail, 320},
}

// maxDecodeMegapixels guards against decompression-bomb uploads — a small
// file that decodes to an enormous pixel grid and exhausts memory. Checked
// against the header (image.DecodeConfig) before the full pixel decode runs.
const maxDecodeMegapixels = 40 * 1_000_000 // 40MP — comfortably above any real phone/DSLR photo

// jpegQuality is the encode quality used for every derived variant. 82 is the
// conventional sweet spot for web delivery: visually indistinguishable from
// the source at normal viewing sizes, at a fraction of the file size of a
// quality-95+ export.
const jpegQuality = 82

// AllowedAspectRatios is the sitewide vocabulary of crop targets. Callers
// pass one of these keys (never a raw float) so an aspect-ratio target is
// always one of a small, deliberately chosen set — never an arbitrary,
// possibly-degenerate value from client input.
var AllowedAspectRatios = map[string]float64{
	"16:9": 16.0 / 9.0,
	"1:1":  1.0,
	"4:5":  4.0 / 5.0,
}

// aspectRatioTolerance is how close a source image's ratio must already be
// to the target before it's left untouched instead of center-cropped. 0.5%
// comfortably absorbs rounding from prior resizes without ever visibly
// cropping an image that's already an exact (or near-exact) match.
const aspectRatioTolerance = 0.005

// ProcessedImage is one derived size: its encoded bytes plus the dimensions
// the caller needs to render an <img> without layout shift.
type ProcessedImage struct {
	Variant     ImageVariantName
	Bytes       []byte
	ContentType string
	Width       int
	Height      int
}

// ProcessedImageSet is the full result of processing one upload: the
// (possibly re-encoded) original plus every derived variant that made sense
// to generate. Variants are never upscaled — if the source is already
// smaller than a rung on the size ladder, that rung is simply omitted and
// callers fall back to the next size up (or the original).
type ProcessedImageSet struct {
	Original       ProcessedImage
	Variants       map[ImageVariantName]ProcessedImage
	OriginalWidth  int
	OriginalHeight int
	// Cropped is true when the source's aspect ratio didn't already match
	// ProcessOptions.TargetAspectRatio and had to be center-cropped to fit.
	// Surfaced so callers can log/observe when server-side enforcement had
	// to actually intervene — a signal that whatever produced this upload
	// didn't already crop to the right shape.
	Cropped bool
}

// ProcessOptions configures one Process call.
type ProcessOptions struct {
	// TargetAspectRatio is width/height (e.g. 16.0/9.0). Zero or negative
	// means "no enforcement" — the source's aspect ratio is preserved
	// exactly as before this option existed.
	TargetAspectRatio float64
}

type ImageProcessor interface {
	// Process decodes raw image bytes, strips metadata (EXIF — including
	// GPS location data — never survives re-encoding through image.Image),
	// auto-corrects orientation, center-crops to opts.TargetAspectRatio if
	// set, and produces the (possibly cropped) original plus every variant
	// in the size ladder that's smaller than the source.
	Process(data []byte, opts ProcessOptions) (*ProcessedImageSet, error)
}

type imageProcessor struct{}

func NewImageProcessor() ImageProcessor {
	return &imageProcessor{}
}

func (p *imageProcessor) Process(data []byte, opts ProcessOptions) (*ProcessedImageSet, error) {
	if len(data) == 0 {
		return nil, errors.New("image data is empty")
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("not a valid image: %w", err)
	}
	if megapixels := int64(cfg.Width) * int64(cfg.Height); megapixels > maxDecodeMegapixels {
		return nil, fmt.Errorf("image dimensions too large (%dx%d)", cfg.Width, cfg.Height)
	}

	// AutoOrientation reads the EXIF orientation tag (common on phone
	// photos shot in portrait) and physically rotates pixels to match, so
	// the re-encoded output — which carries no EXIF at all — still displays
	// right-side up everywhere, including clients that ignore EXIF entirely.
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Crop to the target ratio (if any) before anything is encoded, so the
	// stored original and every derived variant below all come from the
	// same already-correctly-shaped source instead of drifting relative to
	// each other.
	croppedImg, cropped := applyTargetAspectRatio(img, opts.TargetAspectRatio)
	img = croppedImg

	outFormat, contentType := outputFormatFor(format, img)

	encode := func(im image.Image) (ProcessedImage, error) {
		var buf bytes.Buffer
		var encErr error
		if outFormat == imaging.PNG {
			encErr = imaging.Encode(&buf, im, imaging.PNG)
		} else {
			encErr = imaging.Encode(&buf, im, imaging.JPEG, imaging.JPEGQuality(jpegQuality))
		}
		if encErr != nil {
			return ProcessedImage{}, encErr
		}
		bounds := im.Bounds()
		return ProcessedImage{
			Bytes:       buf.Bytes(),
			ContentType: contentType,
			Width:       bounds.Dx(),
			Height:      bounds.Dy(),
		}, nil
	}

	original, err := encode(img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode original: %w", err)
	}
	original.Variant = ""

	set := &ProcessedImageSet{
		Original:       original,
		Variants:       make(map[ImageVariantName]ProcessedImage, len(imageVariantWidths)),
		OriginalWidth:  original.Width,
		OriginalHeight: original.Height,
		Cropped:        cropped,
	}

	// Each step resizes from the previous (smaller) image rather than the
	// original — faster, and since every rung is a downscale of the one
	// before it, visually indistinguishable from resizing the source
	// directly at these output sizes.
	source := img
	for _, rung := range imageVariantWidths {
		if source.Bounds().Dx() <= rung.width {
			// Source is already this size or smaller — never upscale.
			// Callers fall back to the next-largest variant (or Original).
			continue
		}
		resized := imaging.Resize(source, rung.width, 0, imaging.Lanczos)
		variant, err := encode(resized)
		if err != nil {
			return nil, fmt.Errorf("failed to encode %s variant: %w", rung.name, err)
		}
		variant.Variant = rung.name
		set.Variants[rung.name] = variant
		source = resized
	}

	return set, nil
}

// applyTargetAspectRatio center-crops img to the given width/height ratio if
// its current ratio isn't already within aspectRatioTolerance of it. It never
// resizes — only crops — so it never upscales, matching the "never upscale"
// rule the variant ladder already follows. target <= 0 (or a zero-area
// source, which can't happen post-decode but is guarded defensively) is a
// no-op. Returns the possibly-cropped image and whether a crop happened.
func applyTargetAspectRatio(img image.Image, target float64) (image.Image, bool) {
	if target <= 0 {
		return img, false
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return img, false
	}

	current := float64(w) / float64(h)
	if math.Abs(current-target)/target < aspectRatioTolerance {
		return img, false
	}

	var newW, newH int
	if current > target {
		// Wider than target — crop width, keep height.
		newH = h
		newW = int(math.Round(float64(h) * target))
	} else {
		// Taller than target — crop height, keep width.
		newW = w
		newH = int(math.Round(float64(w) / target))
	}

	return imaging.CropCenter(img, newW, newH), true
}

// outputFormatFor decides the re-encode target. PNG sources with an alpha
// channel stay PNG (transparency would be destroyed by JPEG); everything
// else — including WebP and GIF sources, which imaging decodes but does not
// re-encode losslessly in those formats — becomes JPEG, the one format with
// zero compatibility risk across every browser, email client, and social
// share unfurl.
func outputFormatFor(sourceFormat string, img image.Image) (imaging.Format, string) {
	if strings.EqualFold(sourceFormat, "png") && hasAlpha(img) {
		return imaging.PNG, "image/png"
	}
	return imaging.JPEG, "image/jpeg"
}

func hasAlpha(img image.Image) bool {
	switch img.(type) {
	case *image.NRGBA, *image.RGBA, *image.NRGBA64, *image.RGBA64:
		bounds := img.Bounds()
		// Sample a handful of points rather than the whole image — this is
		// a heuristic to avoid a full-image scan on large sources; a false
		// negative just means an occasionally-transparent PNG gets
		// flattened to JPEG, which is a cosmetic edge case, not a bug that
		// corrupts data.
		for _, pt := range samplePoints(bounds) {
			_, _, _, a := img.At(pt.X, pt.Y).RGBA()
			if a < 0xffff {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func samplePoints(bounds image.Rectangle) []image.Point {
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return nil
	}
	return []image.Point{
		{bounds.Min.X, bounds.Min.Y},
		{bounds.Min.X + w/2, bounds.Min.Y + h/2},
		{bounds.Min.X + w - 1, bounds.Min.Y + h - 1},
		{bounds.Min.X, bounds.Min.Y + h - 1},
		{bounds.Min.X + w - 1, bounds.Min.Y},
	}
}
