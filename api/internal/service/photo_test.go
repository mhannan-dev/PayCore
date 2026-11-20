package service

import (
	"context"
	"errors"
	"testing"

	"paycore/api/internal/model"
)

type photoStoreFake struct {
	attachment model.Attachment
	owner      string
	saved      bool
	assigned   *string
}

var errOtherOwner = errors.New("attachment not owned")

func (s *photoStoreFake) Get(_ context.Context, owner, id string) (model.Attachment, error) {
	if owner != s.owner || id != s.attachment.ID {
		return model.Attachment{}, errOtherOwner
	}
	return s.attachment, nil
}
func (s *photoStoreFake) SetPhoto(_ context.Context, _ string, id *string) error {
	s.saved = true
	s.assigned = id
	return nil
}

type photoInspectorFake struct{ info model.ImageInfo }

func (s photoInspectorFake) InspectImage(context.Context, string) (model.ImageInfo, error) {
	return s.info, nil
}

func TestPhotoServicePolicyAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, caller string
		kind         model.AttachmentStorageKind
		info         model.ImageInfo
		valid        bool
	}{
		{"WebP", "owner", "LOCAL", model.ImageInfo{Width: 2, Height: 2, Format: model.ImageWebP}, true},
		{"other owner", "other", "LOCAL", model.ImageInfo{Width: 2, Height: 2, Format: model.ImageWebP}, false},
		{"external", "owner", "EXTERNAL", model.ImageInfo{Width: 2, Height: 2, Format: model.ImagePNG}, false},
		{"oversized", "owner", "LOCAL", model.ImageInfo{Width: 10001, Height: 2, Format: model.ImagePNG}, false},
		{"wrong format", "owner", "LOCAL", model.ImageInfo{Width: 2, Height: 2, Format: model.ImageFormat("svg")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, id := "key", "attachment"
			store := &photoStoreFake{owner: "owner", attachment: model.Attachment{ID: id, StorageKind: tc.kind, StorageKey: &key}}
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
