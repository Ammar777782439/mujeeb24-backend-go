package ports

import (
	"context"
	"time"
)

// Catalog media is a separate bounded context. Commercial offer prices and
// availability stay exclusively in the existing catalog/offer repositories.
type MediaAsset struct {
	ID              string
	BusinessID      string
	Purpose         string
	StorageProvider string
	StorageKey      string
	MimeType        string
	SizeBytes       int64
	SHA256          []byte
	Width           int
	Height          int
	Status          string
	UploadExpiresAt *time.Time
	DeletedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PendingMediaAsset struct {
	ID              string
	BusinessID      string
	StorageKey      string
	UploadExpiresAt time.Time
	CreatedBy       *string
}

type VerifiedImage struct {
	MimeType  string
	SizeBytes int64
	SHA256    []byte
	Width     int
	Height    int
}

type CatalogMediaLink struct {
	ID              string
	BusinessID      string
	CatalogItemID   string
	VariantID       *string
	MediaID         string
	IsPrimary       bool
	SortOrder       int
	AltText         string
	Status          string
	ApprovedAt      *time.Time
	ResourceVersion int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CatalogMediaAttach struct {
	ID            string
	BusinessID    string
	CatalogItemID string
	VariantID     *string
	MediaID       string
	SortOrder     int
	AltText       string
	ApprovedBy    *string
	ApprovedAt    time.Time
}

// Methods that modify a gallery must be invoked inside the caller's database
// transaction, under LockGallery. No network I/O belongs in the transaction.
type CatalogMediaRepository interface {
	CreatePendingAsset(context.Context, PendingMediaAsset) (MediaAsset, error)
	GetAsset(context.Context, string, string) (MediaAsset, error)
	MarkAssetReady(context.Context, string, string, VerifiedImage) (MediaAsset, error)
	LockGallery(context.Context, string, string, *string) error
	ListGallery(context.Context, string, string, *string) ([]CatalogMediaLink, error)
	Attach(context.Context, CatalogMediaAttach, bool) (CatalogMediaLink, error)
	SetPrimary(context.Context, string, string, string, *string, string) error
	Remove(context.Context, string, string, string, *string, string) error
	PrimaryForReply(context.Context, string, string, *string) (*CatalogMediaLink, error)
	FindApprovedMediaLink(context.Context,string,string)(*CatalogMediaLink,error)
}


// MediaObjectStorage is private storage; signed read URLs are generated only
// at outbound dispatch and never stored in media_assets or message records.
type MediaObjectStorage interface {
    Put(context.Context,string,[]byte) error
    Delete(context.Context,string) error
    PresignGet(context.Context,string,time.Duration)(string,error)
}
