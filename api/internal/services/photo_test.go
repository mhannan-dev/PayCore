package services

import (
	"context"
	"errors"
	"testing"

	"paycore/api/internal/models"
)

type photoStoreFake struct {
	attachment models.Attachment
	owner      string
	saved      bool
	assigned   *string
}

var errOtherOwner = errors.New("attachment not owned")

func (s *photoStoreFake) Get(_ context.Context, owner, id string) (models.Attachment, error) {
	if owner != s.owner || id != s.attachment.ID {
		return models.Attachment{}, errOtherOwner
	}
	return s.attachment, nil
}
func (s *photoStoreFake) SetPhoto(_ context.Context, _ string, id *string) error {
	s.saved = true
	s.assigned = id
	return nil
}

type photoInspectorFake struct{ info models.ImageInfo }

func (s photoInspectorFake) InspectImage(context.Context, string) (models.ImageInfo, error) {
	return s.info, nil
}

func TestPhotoServicePolicyAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, caller string
		kind         models.AttachmentStorageKind
		info         models.ImageInfo
		valid        bool
	}{
		{"WebP", "owner", "LOCAL", models.ImageInfo{Width: 2, Height: 2, Format: models.ImageWebP}, true},
		{"other owner", "other", "LOCAL", models.ImageInfo{Width: 2, Height: 2, Format: models.ImageWebP}, false},
		{"external", "owner", "EXTERNAL", models.ImageInfo{Width: 2, Height: 2, Format: models.ImagePNG}, false},
		{"oversized", "owner", "LOCAL", models.ImageInfo{Width: 10001, Height: 2, Format: models.ImagePNG}, false},
		{"wrong format", "owner", "LOCAL", models.ImageInfo{Width: 2, Height: 2, Format: models.ImageFormat("svg")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, id := "key", "attachment"
			store := &photoStoreFake{owner: "owner", attachment: models.Attachment{ID: id, StorageKind: tc.kind, StorageKey: &key}}
			svc := NewPhotoService(store, photoInspectorFake{info: tc.info})
			result, err := svc.Set(context.Background(), tc.caller, &id)
			if tc.valid {
				if err != nil || !store.saved || result.AttachmentID == nil || *result.AttachmentID != id || result.Photo == nil {
					t.Fatalf("valid photo rejected: %v", err)
				}
			} else if err == nil || store.saved {
				t.Fatal("invalid photo was persisted")
			}
			store.saved = false
			result, err = svc.Set(context.Background(), tc.caller, nil)
			if err != nil || !store.saved || result.AttachmentID != nil || result.Photo != nil {
				t.Fatal("clear must not inspect or remove attachment bytes")
			}
		})
	}
}
