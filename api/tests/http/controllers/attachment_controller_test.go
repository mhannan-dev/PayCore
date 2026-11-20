package controllers_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"paycore/api/internal/helpers"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/database/migrations"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/server"
	"paycore/api/internal/services"
	"paycore/api/internal/storage"
)

func TestAttachmentsIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		t.Skip("set DATABASE_TEST_URL for isolated attachment API integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "attachments_test_" + hex.EncodeToString(random[:])
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error("test schema cleanup failed")
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := repositories.NewUserRepository(pool)
	hash, err := services.HashPassword("Attach#t01")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := users.Create(ctx, "Owner", nil, "owner@example.test", hash, []string{"OPERATOR"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.Create(ctx, "Other", nil, "other@example.test", hash, []string{"ADMIN"})
	if err != nil {
		t.Fatal(err)
	}
	ownerToken, otherToken := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for token, userID := range map[string]string{ownerToken: owner.ID, otherToken: other.ID} {
		digest := sha256.Sum256([]byte(token))
		if userID == other.ID {
			// Fixture represents an admin who has already completed MFA.
			if _, err = pool.Exec(ctx, "INSERT INTO auth_sessions(token_hash,user_id,expires_at,mfa_verified) VALUES($1,$2,$3,TRUE)", digest[:], userID, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err = users.CreateSession(ctx, userID, hash, digest[:], time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := services.NewAuthService(users, nil)
	if err != nil {
		t.Fatal(err)
	}
	uploadDirectory := t.TempDir()
	files, err := storage.OpenLocal(uploadDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	repo := repositories.NewAttachmentRepository(pool)
	router := server.New(pool, server.Controllers{AuthMiddleware: middleware.NewAuth(auth), Auth: controllers.NewAuthController(auth, nil), Attachments: controllers.NewAttachmentController(services.NewAttachmentService(repo, files, helpers.NewWebPConverter(2)), services.NewPhotoService(repo, files))})
	request := func(method, path, token, contentType string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	upload := func(name string, contents []byte) models.Attachment {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(contents); err != nil {
			t.Fatal(err)
		}
		writer.Close()
		w := request("POST", "/api/v1/attachments", ownerToken, writer.FormDataContentType(), body.Bytes())
		if w.Code != http.StatusCreated {
			t.Fatalf("upload failed: status %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "storage_key") {
			t.Fatal("filesystem key exposed")
		}
		var a models.Attachment
		if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	if w := request("GET", "/api/v1/attachments", "", "", nil); w.Code != 401 {
		t.Fatal("anonymous access accepted")
	}
	document := upload("../../invoice.html", []byte("<html><script>alert('sample')</script></html>"))
	if document.FileName != "invoice.html" {
		t.Fatal("filename not normalized")
	}
	contentPath := "/api/v1/attachments/" + document.ID + "/content"
	if w := request("GET", contentPath, otherToken, "", nil); w.Code != 404 {
		t.Fatal("cross-user download allowed")
	}
	if w := request("DELETE", "/api/v1/attachments/"+document.ID, otherToken, "", nil); w.Code != 404 {
		t.Fatal("cross-user delete allowed")
	}
	w := request("GET", contentPath, ownerToken, "", nil)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("unsafe download headers")
	}
	if w := request("PUT", "/api/v1/auth/photo", ownerToken, "application/json", []byte(`{"attachment_id":"`+document.ID+`"}`)); w.Code != 415 {
		t.Fatal("non-image accepted as photo")
	}
	var picture bytes.Buffer
	if err = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	photo := upload("avatar.png", picture.Bytes())
	if w := request("PUT", "/api/v1/auth/photo", otherToken, "application/json", []byte(`{"attachment_id":"`+photo.ID+`"}`)); w.Code != 404 {
		t.Fatal("cross-user photo assignment allowed")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET photo_attachment_id=$1 WHERE id=$2", photo.ID, other.ID); err == nil {
		t.Fatal("database allowed cross-user photo FK")
	}
	if w := request("PUT", "/api/v1/auth/photo", ownerToken, "application/json", []byte(`{"attachment_id":"`+photo.ID+`"}`)); w.Code != 200 {
		t.Fatalf("photo assignment failed: %d", w.Code)
	}
	w = request("GET", "/api/v1/auth/me", ownerToken, "", nil)
	var me models.UserDTO
	if err = json.Unmarshal(w.Body.Bytes(), &me); err != nil || me.PhotoAttachmentID == nil || *me.PhotoAttachmentID != photo.ID || me.Photo == nil {
		t.Fatal("profile did not reference attachment")
	}
	w = request("GET", "/api/v1/auth/photo", ownerToken, "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/webp" {
		t.Fatal("photo content failed")
	}
	if decoded, format, err := image.Decode(bytes.NewReader(w.Body.Bytes())); err != nil || format != "webp" || decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 2 {
		t.Fatalf("converted profile photo invalid: %s %v", format, err)
	}
	if w := request("PUT", "/api/v1/auth/photo", ownerToken, "application/json", []byte(`{}`)); w.Code != 400 {
		t.Fatal("missing photo field silently cleared link")
	}
	if w := request("PUT", "/api/v1/auth/photo", ownerToken, "application/json", []byte(`{"attachment_id":null}`)); w.Code != 200 {
		t.Fatal("clear photo failed")
	}
	if w := request("PUT", "/api/v1/auth/photo", ownerToken, "application/json", []byte(`{"attachment_id":"`+photo.ID+`"}`)); w.Code != 200 {
		t.Fatal("reassign photo failed")
	}
	if w := request("DELETE", "/api/v1/attachments/"+photo.ID, ownerToken, "", nil); w.Code != 204 {
		t.Fatal("photo attachment delete failed")
	}
	if w := request("GET", "/api/v1/auth/photo", ownerToken, "", nil); w.Code != 404 {
		t.Fatal("deleted photo still available")
	}
	u, err := users.FindByEmail(ctx, owner.Email)
	if err != nil || u.PhotoAttachmentID != nil || u.Photo != nil {
		t.Fatal("photo link not cleared on delete")
	}
	if w := request("GET", "/api/v1/attachments/"+photo.ID, ownerToken, "", nil); w.Code != 404 {
		t.Fatal("deleted metadata still available")
	}
	if w := request("GET", "/api/v1/attachments?limit=1000", ownerToken, "", nil); w.Code != 400 {
		t.Fatal("unbounded list accepted")
	}
	if w := request("GET", "/api/v1/attachments/not-a-uuid", ownerToken, "", nil); w.Code != 400 {
		t.Fatal("invalid attachment id accepted")
	}
	for _, tc := range []struct {
		name, content string
		count, status int
		code          string
	}{
		{"empty.txt", "", 1, 413, "file_size_must_be_1_byte_to_10_mib"},
		{"vector.svg", "<svg></svg>", 1, 415, "unsupported_image"},
		{"multiple.txt", "text", 2, 400, "one_file_required"},
	} {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for i := 0; i < tc.count; i++ {
			part, err := form.CreateFormFile("file", tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = part.Write([]byte(tc.content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		w := request("POST", "/api/v1/attachments", ownerToken, form.FormDataContentType(), body.Bytes())
		if w.Code != tc.status || errorCode(t, w) != tc.code {
			t.Fatalf("validation response for %s: %d %s", tc.name, w.Code, w.Body)
		}
	}
	// A definite database failure must remove the bytes already written to disk.
	before, err := os.ReadDir(uploadDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_attachment() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture metadata failure'; END $$;
	 CREATE TRIGGER reject_attachment BEFORE INSERT ON attachments FOR EACH ROW EXECUTE FUNCTION reject_attachment()`); err != nil {
		t.Fatal(err)
	}
	var rejected bytes.Buffer
	writer := multipart.NewWriter(&rejected)
	part, err := writer.CreateFormFile("file", "rejected.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("do not retain this upload")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/api/v1/attachments", ownerToken, writer.FormDataContentType(), rejected.Bytes())
	if w.Code != 500 || strings.Contains(w.Body.String(), "fixture metadata failure") {
		t.Fatalf("metadata failure response: %d %s", w.Code, w.Body)
	}
	after, err := os.ReadDir(uploadDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatal("database rejection left an orphan upload")
	}
}
