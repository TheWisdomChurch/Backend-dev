package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// ErrVideoToolingUnavailable means ffmpeg/ffprobe are not installed on this
// host. Callers should treat this as a soft failure — fall back to storing
// the raw upload rather than rejecting the request outright, since a missing
// binary is an environment problem, not a bad upload.
var ErrVideoToolingUnavailable = errors.New("video processing tools (ffmpeg/ffprobe) are not installed")

// VideoMetadata is what ffprobe tells us about an uploaded video — enough to
// validate it's a real, playable video and to size a poster frame / player
// UI without waiting for the transcode to finish.
type VideoMetadata struct {
	DurationSeconds float64
	Width           int
	Height          int
	VideoCodec      string
	AudioCodec      string
}

type VideoProcessor interface {
	// Available reports whether ffmpeg/ffprobe were found on this host at
	// construction time. Callers check this once up front to decide whether
	// to run the full pipeline or fall back to plain storage passthrough.
	Available() bool

	// Probe extracts real metadata from the file at path and, by virtue of
	// requiring ffprobe to successfully parse it, validates the file is an
	// actual decodable video — not just something with a video/* extension.
	Probe(ctx context.Context, path string) (*VideoMetadata, error)

	// ExtractPosterFrame grabs a single frame (fixed offset into the clip,
	// clamped to the actual duration) and writes it as JPEG bytes — the
	// thumbnail shown before playback starts, the same pattern every video
	// product uses instead of a blank/black first frame.
	ExtractPosterFrame(ctx context.Context, inputPath string, duration float64) ([]byte, error)

	// Transcode re-encodes to a single, predictable web-delivery format
	// (H.264/AAC MP4, capped resolution, faststart) regardless of what
	// container/codec the source used — so playback is consistent across
	// every browser and device instead of depending on whatever the
	// visitor's phone happened to export.
	Transcode(ctx context.Context, inputPath, outputPath string) error
}

const (
	// maxTranscodeWidth caps output resolution at 1080p-class — enough for
	// full-screen playback on virtually any device, without inflating
	// storage/bandwidth for 4K phone footage nobody will view at full res.
	maxTranscodeWidth = 1920
	// videoCRF (Constant Rate Factor) controls the quality/size trade-off
	// for libx264. 23 is the widely-used "visually lossless for web" default.
	videoCRF = "23"
)

type videoProcessor struct {
	available   bool
	ffmpegPath  string
	ffprobePath string
}

func NewVideoProcessor() VideoProcessor {
	ffmpegPath, ffErr := exec.LookPath("ffmpeg")
	ffprobePath, fpErr := exec.LookPath("ffprobe")
	return &videoProcessor{
		available:   ffErr == nil && fpErr == nil,
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
	}
}

func (p *videoProcessor) Available() bool {
	return p != nil && p.available
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

func (p *videoProcessor) Probe(ctx context.Context, path string) (*VideoMetadata, error) {
	if !p.Available() {
		return nil, ErrVideoToolingUnavailable
	}

	cmd := exec.CommandContext(ctx, p.ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe failed (not a valid video): %s: %w", stderr.String(), err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	meta := &VideoMetadata{}
	if d, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
		meta.DurationSeconds = d
	}
	hasVideoStream := false
	for _, s := range parsed.Streams {
		switch s.CodecType {
		case "video":
			hasVideoStream = true
			meta.VideoCodec = s.CodecName
			meta.Width = s.Width
			meta.Height = s.Height
		case "audio":
			meta.AudioCodec = s.CodecName
		}
	}
	if !hasVideoStream {
		return nil, errors.New("file has no video stream — not a valid video")
	}

	return meta, nil
}

func (p *videoProcessor) ExtractPosterFrame(ctx context.Context, inputPath string, duration float64) ([]byte, error) {
	if !p.Available() {
		return nil, ErrVideoToolingUnavailable
	}

	// Seek to 10% into the clip (capped at 3s) rather than frame zero, which
	// on many clips is a black/blank transition frame — the same heuristic
	// video platforms use for auto-generated thumbnails.
	seek := duration * 0.1
	if seek > 3 {
		seek = 3
	}
	if seek < 0 {
		seek = 0
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, p.ffmpegPath,
		"-y",
		"-ss", fmt.Sprintf("%.2f", seek),
		"-i", inputPath,
		"-frames:v", "1",
		"-q:v", "2",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("poster frame extraction failed: %s: %w", stderr.String(), err)
	}
	if stdout.Len() == 0 {
		return nil, errors.New("poster frame extraction produced no output")
	}
	return stdout.Bytes(), nil
}

func (p *videoProcessor) Transcode(ctx context.Context, inputPath, outputPath string) error {
	if !p.Available() {
		return ErrVideoToolingUnavailable
	}

	cmd := exec.CommandContext(ctx, p.ffmpegPath,
		"-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", videoCRF,
		// Downscale only if wider than the cap; -2 keeps height even
		// (required by libx264's 4:2:0 chroma subsampling).
		"-vf", fmt.Sprintf("scale='min(%d,iw)':-2", maxTranscodeWidth),
		"-c:a", "aac",
		"-b:a", "128k",
		// Moves the MP4 index to the front of the file so playback can
		// start before the whole file downloads — without this, every
		// video needs a full fetch before a browser can show frame one.
		"-movflags", "+faststart",
		outputPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("transcode failed: %s: %w", stderr.String(), err)
	}
	return nil
}
