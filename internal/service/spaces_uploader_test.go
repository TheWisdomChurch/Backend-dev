package service

import "testing"

func TestDerivePublicBaseURLAWSWithoutEndpoint(t *testing.T) {
	got := derivePublicBaseURL("wisdom-media", "", "eu-west-2", "aws")
	want := "https://wisdom-media.s3.eu-west-2.amazonaws.com"

	if got != want {
		t.Fatalf("derivePublicBaseURL() = %q, want %q", got, want)
	}
}

func TestDerivePublicBaseURLSupabase(t *testing.T) {
	got := derivePublicBaseURL(
		"media",
		"https://project-ref.storage.supabase.co/storage/v1/s3",
		"us-east-1",
		"supabase",
	)
	want := "https://project-ref.supabase.co/storage/v1/object/public/media"

	if got != want {
		t.Fatalf("derivePublicBaseURL() = %q, want %q", got, want)
	}
}

func TestShouldForcePathStyle(t *testing.T) {
	t.Run("aws provider uses virtual hosted style", func(t *testing.T) {
		t.Setenv("S3_FORCE_PATH_STYLE", "")

		if shouldForcePathStyle("aws", "") {
			t.Fatal("expected aws provider to default to virtual hosted style")
		}
	})

	t.Run("compatible endpoint uses path style", func(t *testing.T) {
		t.Setenv("S3_FORCE_PATH_STYLE", "")

		if !shouldForcePathStyle("supabase", "https://project-ref.storage.supabase.co/storage/v1/s3") {
			t.Fatal("expected compatible endpoint to default to path style")
		}
	})

	t.Run("explicit env wins", func(t *testing.T) {
		t.Setenv("S3_FORCE_PATH_STYLE", "false")

		if shouldForcePathStyle("minio", "https://storage.example.com") {
			t.Fatal("expected S3_FORCE_PATH_STYLE=false to disable path style")
		}
	})
}

func TestBuildGenericAssetKeyKeepsConfiguredBasePath(t *testing.T) {
	uploader := &S3Uploader{basePath: "production/media"}

	got, err := uploader.BuildGenericAssetKey("Admin Uploads/Image Banners", "PNG")
	if err != nil {
		t.Fatalf("BuildGenericAssetKey returned error: %v", err)
	}

	if wantPrefix := "production/media/admin-uploads/image-banners/"; len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("BuildGenericAssetKey() = %q, want prefix %q", got, wantPrefix)
	}

	if got[len(got)-4:] != ".png" {
		t.Fatalf("BuildGenericAssetKey() = %q, want .png suffix", got)
	}
}
