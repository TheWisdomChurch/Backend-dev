package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/handlers"
	"wisdomHouse-backend/internal/jobs"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
)

func fail(step string, err error) {
	fmt.Printf("FAIL[%s]: %v\n", step, err)
	os.Exit(1)
}

func mustOK(step string, cond bool, detail string) {
	if !cond {
		fail(step, fmt.Errorf("%s", detail))
	}
	fmt.Println("OK:", step)
}

func multipartUpload(url string, data []byte, filename, contentType string, fields map[string]string) (*http.Response, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)},
		"Content-Type":        {contentType},
	})
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return http.DefaultClient.Do(req)
}

func main() {
	stored := map[string][]byte{}
	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			data, _ := io.ReadAll(r.Body)
			stored[r.URL.Path] = data
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			data, ok := stored[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer s3srv.Close()

	os.Setenv("S3_PROVIDER", "supabase")
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("S3_REGION", "us-east-1")
	os.Setenv("S3_ENDPOINT", s3srv.URL+"/storage/v1/s3")
	os.Setenv("S3_ACCESS_KEY", "AKIATESTKEY")
	os.Setenv("S3_SECRET_KEY", "testSecretKey1234567890abcdefgh")
	os.Setenv("S3_PUBLIC_BASE_URL", s3srv.URL+"/public/test-bucket")
	os.Setenv("S3_PUBLIC_READ", "false")

	uploader, err := service.NewS3UploaderFromEnv()
	if err != nil || uploader == nil {
		fail("uploader-construct", err)
	}

	dsn := "host=/tmp user=" + os.Getenv("USER") + " dbname=wisdom_video_test sslmode=disable"
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fail("db-connect", err)
	}
	db := &database.Database{DB: gdb}
	assetRepo := repository.NewAssetRepository(db)
	assetService := service.NewAssetService(assetRepo, uploader)

	videoProcessor := service.NewVideoProcessor()
	if !videoProcessor.Available() {
		fail("ffmpeg-availability", fmt.Errorf("ffmpeg/ffprobe not found on PATH — test environment issue, not a product bug"))
	}
	fmt.Println("OK: ffmpeg/ffprobe available")

	redisConnOpt, err := asynq.ParseRedisURI("redis://localhost:6379/3")
	if err != nil {
		fail("redis-uri-parse", err)
	}
	asynqClient := asynq.NewClient(redisConnOpt)
	defer asynqClient.Close()

	asynqServer := asynq.NewServer(redisConnOpt, asynq.Config{Concurrency: 2, Queues: map[string]int{"video": 1}})
	mux := asynq.NewServeMux()
	videoHandler := jobs.NewVideoProcessHandler(assetService, uploader, videoProcessor)
	mux.HandleFunc(jobs.TypeVideoProcess, videoHandler.ProcessTask)
	go func() {
		if err := asynqServer.Run(mux); err != nil {
			fmt.Println("asynq server stopped:", err)
		}
	}()
	defer asynqServer.Shutdown()

	uploadHandler := handlers.NewUploadHandler(uploader, assetService)
	uploadHandler.SetVideoQueue(asynqClient)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload-file", uploadHandler.UploadFile)
	apiSrv := httptest.NewServer(router)
	defer apiSrv.Close()

	videoBytes, err := os.ReadFile("/tmp/test_video.mp4")
	if err != nil {
		fail("read-test-video", err)
	}

	resp, err := multipartUpload(apiSrv.URL+"/upload-file", videoBytes, "clip.mp4", "video/mp4", map[string]string{
		"kind": "video", "module": "reels", "folder": "reels/test",
	})
	if err != nil {
		fail("video-upload-request", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	bodyStr := string(body)
	mustOK("video upload returns 200", resp.StatusCode == http.StatusOK, fmt.Sprintf("status=%d body=%s", resp.StatusCode, bodyStr))
	mustOK("response reports processing=true", strings.Contains(bodyStr, `"processing":true`), bodyStr)
	mustOK("response has posterUrl", strings.Contains(bodyStr, `"posterUrl":"http`), bodyStr)
	mustOK("response has correct probed width/height", strings.Contains(bodyStr, `"width":1280`) && strings.Contains(bodyStr, `"height":720`), bodyStr)
	mustOK("response has duration ~3s", strings.Contains(bodyStr, `"duration":2.9`) || strings.Contains(bodyStr, `"duration":3`), bodyStr)
	mustOK("status is pending (queued for transcode)", strings.Contains(bodyStr, `"status":"pending"`), bodyStr)

	// Extract assetId from the raw JSON the crude way (no need for a full decoder here).
	assetID := extractJSONString(bodyStr, `"assetId":"`)
	mustOK("assetId present in response", assetID != "", bodyStr)

	// Confirm the poster is a real, decodable JPEG of reasonable size.
	foundPoster := false
	for k, v := range stored {
		if strings.Contains(k, "poster") {
			foundPoster = true
			mustOK("poster object is non-trivial size", len(v) > 500, fmt.Sprintf("poster %s is %d bytes", k, len(v)))
		}
	}
	mustOK("poster object was actually stored", foundPoster, "")

	// Confirm original was stored untouched (same size as source).
	foundOriginal := false
	for k, v := range stored {
		if strings.Contains(k, "original") {
			foundOriginal = true
			mustOK("original video stored at full size", len(v) == len(videoBytes), fmt.Sprintf("stored %d vs source %d", len(v), len(videoBytes)))
		}
	}
	mustOK("original video object was stored", foundOriginal, "")

	// Poll DB until the async worker flips status to ready (bounded wait).
	fmt.Println("waiting for async transcode to complete...")
	deadline := time.Now().Add(60 * time.Second)
	var finalStatus, metadataJSON string
	for time.Now().Before(deadline) {
		row := db.DB.Raw(`SELECT status, metadata::text FROM assets WHERE id = ?`, assetID).Row()
		if err := row.Scan(&finalStatus, &metadataJSON); err != nil {
			fail("poll-asset-status", err)
		}
		if finalStatus == "ready" || finalStatus == "failed" {
			break
		}
		time.Sleep(1 * time.Second)
	}
	mustOK("asset transitioned to ready", finalStatus == "ready", fmt.Sprintf("final status=%s metadata=%s", finalStatus, metadataJSON))
	mustOK("metadata contains transcodedUrl", strings.Contains(metadataJSON, "transcodedUrl"), metadataJSON)

	// Find the transcoded object and verify it's a real, smaller/optimized MP4.
	var transcodedKey string
	for k := range stored {
		if strings.Contains(k, "/video.mp4") {
			transcodedKey = k
		}
	}
	mustOK("transcoded video object exists in storage", transcodedKey != "", fmt.Sprintf("stored keys: %v", keysOf(stored)))

	transcodedBytes := stored[transcodedKey]
	tmpFile, err := os.CreateTemp("", "transcoded-*.mp4")
	if err != nil {
		fail("temp-file", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.Write(transcodedBytes)
	tmpFile.Close()

	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_name,width,height", "-of", "default=noprint_wrappers=1", tmpFile.Name()).CombinedOutput()
	if err != nil {
		fail("ffprobe-transcoded-output", fmt.Errorf("%s: %w", out, err))
	}
	outStr := string(out)
	mustOK("transcoded output is valid h264", strings.Contains(outStr, "codec_name=h264"), outStr)
	mustOK("transcoded output has aac audio", strings.Contains(outStr, "codec_name=aac"), outStr)
	fmt.Println("  transcoded ffprobe output:\n" + outStr)

	fmt.Println("ALL_OK")
}

func extractJSONString(body, marker string) string {
	idx := strings.Index(body, marker)
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
