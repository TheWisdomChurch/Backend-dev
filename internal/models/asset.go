package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AssetStatus string

const (
	AssetStatusPending AssetStatus = "pending"
	AssetStatusReady   AssetStatus = "ready"
	AssetStatusFailed  AssetStatus = "failed"
)

// Asset represents an uploaded file stored in external object storage.
type Asset struct {
	ID string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`

	OwnerType *string `gorm:"size:50;index" json:"ownerType,omitempty"`
	OwnerID   *string `gorm:"type:uuid;index" json:"ownerId,omitempty"`
	Kind      *string `gorm:"size:50;index" json:"kind,omitempty"`

	Provider  string `gorm:"size:50;not null" json:"provider"`
	Bucket    string `gorm:"size:255;not null" json:"bucket"`
	ObjectKey string `gorm:"type:text;not null;uniqueIndex" json:"objectKey"`
	PublicURL string `gorm:"type:text;not null" json:"publicUrl"`

	ContentType *string `gorm:"size:255" json:"contentType,omitempty"`
	SizeBytes   *int64  `json:"sizeBytes,omitempty"`
	Checksum    *string `gorm:"size:128" json:"checksum,omitempty"`

	Status   AssetStatus    `gorm:"size:20;not null;default:'pending'" json:"status"`
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`

	CreatedByID *string `gorm:"type:uuid" json:"createdById,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Asset) TableName() string {
	return "assets"
}

type PresignAssetRequest struct {
	OwnerType   *string `json:"ownerType,omitempty"`
	OwnerID     *string `json:"ownerId,omitempty"`
	Kind        *string `json:"kind,omitempty"`
	Folder      *string `json:"folder,omitempty"`
	ContentType string  `json:"contentType" binding:"required"`
	SizeBytes   *int64  `json:"sizeBytes,omitempty"`
	Checksum    *string `json:"checksum,omitempty"`
}

type PresignAssetResponse struct {
	AssetID   string `json:"assetId"`
	UploadURL string `json:"uploadUrl"`
	ObjectKey string `json:"objectKey"`
	PublicURL string `json:"publicUrl"`
}

type RecordUploadedAssetRequest struct {
	OwnerType    *string `json:"ownerType,omitempty"`
	OwnerID      *string `json:"ownerId,omitempty"`
	Kind         *string `json:"kind,omitempty"`
	Folder       *string `json:"folder,omitempty"`
	ObjectKey    string  `json:"objectKey"`
	PublicURL    string  `json:"publicUrl"`
	ContentType  string  `json:"contentType"`
	SizeBytes    int64   `json:"sizeBytes"`
	Checksum     *string `json:"checksum,omitempty"`
	OriginalName *string `json:"originalName,omitempty"`
	// Metadata carries caller-supplied extras (e.g. image dimensions and
	// derived-variant URLs) merged into the stored asset's Metadata JSONB
	// alongside the folder/originalName/ownerID bookkeeping the service
	// already tracks.
	Metadata map[string]any `json:"metadata,omitempty"`
}

type UploadedAssetResponse struct {
	ID           string `json:"id"`
	AssetID      string `json:"assetId"`
	URL          string `json:"url"`
	PublicURL    string `json:"publicUrl"`
	ObjectKey    string `json:"objectKey"`
	Key          string `json:"key"`
	Bucket       string `json:"bucket"`
	Provider     string `json:"provider"`
	ContentType  string `json:"contentType"`
	MimeType     string `json:"mimeType"`
	SizeBytes    int64  `json:"sizeBytes"`
	Kind         string `json:"kind,omitempty"`
	OwnerType    string `json:"ownerType,omitempty"`
	OwnerID      string `json:"ownerId,omitempty"`
	Folder       string `json:"folder,omitempty"`
	OriginalName string `json:"originalName,omitempty"`
	Status       string `json:"status"`
}
