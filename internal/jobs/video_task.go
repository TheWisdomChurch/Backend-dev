// Package jobs holds background task definitions processed by the in-process
// asynq worker started in cmd/api/main.go. Keeping the worker in the same
// binary as the API avoids standing up a second deployable service — a
// deliberate scope decision for an app this size — while still getting the
// two things that actually matter for video processing: the HTTP request
// never blocks on a multi-second/minute transcode, and a queued job survives
// an API restart instead of being silently dropped.
package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hibiken/asynq"

	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
)

// TypeVideoProcess is the asynq task type name for video transcoding.
const TypeVideoProcess = "video:process"

// VideoProcessPayload is everything the worker needs to do its job without
// re-deriving anything from the asset row — the request handler already
// knows the folder/assetID it used when it stored the original.
type VideoProcessPayload struct {
	AssetID   string `json:"assetId"`
	Folder    string `json:"folder"`
	SourceURL string `json:"sourceUrl"`
}

// NewVideoProcessTask builds the asynq task enqueued right after a video
// upload's original bytes and poster frame are safely stored.
func NewVideoProcessTask(payload VideoProcessPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal video process payload: %w", err)
	}
	return asynq.NewTask(TypeVideoProcess, data,
		asynq.MaxRetry(3),
		asynq.Timeout(15*time.Minute),
		asynq.Queue("video"),
	), nil
}

// VideoProcessHandler transcodes one queued video. Registered against the
// asynq ServeMux in cmd/api/main.go.
type VideoProcessHandler struct {
	assets    service.AssetService
	uploader  service.AssetUploader
	processor service.VideoProcessor
}

func NewVideoProcessHandler(assets service.AssetService, uploader service.AssetUploader, processor service.VideoProcessor) *VideoProcessHandler {
	return &VideoProcessHandler{assets: assets, uploader: uploader, processor: processor}
}

func (h *VideoProcessHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload VideoProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		// A malformed payload will never succeed on retry — asynq treats a
		// SkipRetry-wrapped error as terminal instead of burning retries.
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	log := applog.L().With("asset_id", payload.AssetID)

	inputFile, err := h.downloadToTemp(ctx, payload.SourceURL)
	if err != nil {
		h.markFailed(payload.AssetID, err)
		return fmt.Errorf("download source video: %w", err)
	}
	defer os.Remove(inputFile)

	outputFile, err := os.CreateTemp("", "wisdom-video-out-*.mp4")
	if err != nil {
		h.markFailed(payload.AssetID, err)
		return fmt.Errorf("create temp output: %w", err)
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer os.Remove(outputPath)

	if err := h.processor.Transcode(ctx, inputFile, outputPath); err != nil {
		h.markFailed(payload.AssetID, err)
		return fmt.Errorf("transcode: %w", err)
	}

	meta, probeErr := h.processor.Probe(ctx, outputPath)
	if probeErr != nil {
		log.Warn("post-transcode probe failed, proceeding without refreshed dimensions", "error", probeErr)
	}

	outBytes, err := os.ReadFile(outputPath)
	if err != nil {
		h.markFailed(payload.AssetID, err)
		return fmt.Errorf("read transcoded output: %w", err)
	}

	key, err := h.uploader.BuildImageVariantKey(payload.Folder, payload.AssetID, "video", "mp4")
	if err != nil {
		h.markFailed(payload.AssetID, err)
		return fmt.Errorf("build storage key: %w", err)
	}

	uploadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	transcodedURL, err := h.uploader.Upload(uploadCtx, key, "video/mp4", bytes.NewReader(outBytes))
	if err != nil {
		h.markFailed(payload.AssetID, err)
		return fmt.Errorf("upload transcoded output: %w", err)
	}

	patch := map[string]any{
		"transcodedUrl": transcodedURL,
		"processedAt":   time.Now().UTC().Format(time.RFC3339),
	}
	if meta != nil {
		patch["width"] = meta.Width
		patch["height"] = meta.Height
		patch["duration"] = meta.DurationSeconds
	}

	if err := h.assets.UpdateProcessingResult(payload.AssetID, models.AssetStatusReady, patch); err != nil {
		return fmt.Errorf("record processing result: %w", err)
	}

	log.Info("video processed", "transcoded_url", transcodedURL, "size_bytes", len(outBytes))
	return nil
}

func (h *VideoProcessHandler) markFailed(assetID string, cause error) {
	patch := map[string]any{"processingError": cause.Error()}
	if err := h.assets.UpdateProcessingResult(assetID, models.AssetStatusFailed, patch); err != nil {
		applog.L().Warn("failed to record video processing failure", "asset_id", assetID, "error", err)
	}
}

func (h *VideoProcessHandler) downloadToTemp(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status fetching source video: %d", resp.StatusCode)
	}

	f, err := os.CreateTemp("", "wisdom-video-in-*")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
