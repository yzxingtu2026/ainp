package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	systemjobs "github.com/yzxingtu2026/ainp/platform/modules/system/jobs"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestAttachmentThumbSkippedAndVariantGate(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_thumb_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&tenant.Tenant{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&model.TenantAccount{
		ID: "user-a", TenantID: "tenant-a", Username: "admin-a", PasswordHash: "x", Status: status.ActivationEnabled,
	}).Error; err != nil {
		t.Fatal(err)
	}

	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewAttachmentService(testDB, application.Storage, application.TransferTokens, application.AttachmentPolicies, nil)

	perms := []string{system.ActionAttachmentUploadPrepare, system.ActionAttachmentDownloadPrepare, system.ActionAttachmentDelete}
	ctxA := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})
	ctxB := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-b", PrincipalID: "user-b", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})

	payload := []byte("plain-text-no-thumb")
	prepared, err := svc.UploadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"original_name":"note.txt","content_type":"text/plain","size":%d}`, len(payload),
	)))
	if err != nil {
		t.Fatal(err)
	}
	prep := prepared.(system.TransferPrepareView)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	system.RegisterAttachmentTransferRoutes(router, svc)
	uploadReq := httptest.NewRequest(http.MethodPut, prep.UploadURL, bytes.NewReader(payload))
	uploadReq.Header.Set("Authorization", "Bearer "+prep.Token)
	uploadResp := httptest.NewRecorder()
	router.ServeHTTP(uploadResp, uploadReq)
	if envelopeCode(uploadResp.Body.Bytes()) != 0 {
		t.Fatalf("upload failed: %s", uploadResp.Body.String())
	}

	tenantDB, err := tenant.TenantDB(ctxA, testDB)
	if err != nil {
		t.Fatal(err)
	}
	attachments, _ := tenant.NewTenantRepository[model.Attachment](tenantDB)
	row, err := attachments.GetByID(prep.Attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.GetThumbStatus() != status.AttachmentThumbSkipped {
		t.Fatalf("thumb_status = %q want skipped", row.GetThumbStatus())
	}
	if _, err := svc.DownloadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"id":%q,"variant":"thumb"}`, prep.Attachment.ID,
	))); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("expected variant_unavailable, err=%v", err)
	}

	// Image upload → pending, then materialize thumb and download.
	imgPayload := mustPNG(t, 320, 240)
	imgPrepRaw, err := svc.UploadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"original_name":"pic.png","content_type":"image/png","size":%d}`, len(imgPayload),
	)))
	if err != nil {
		t.Fatal(err)
	}
	imgPrep := imgPrepRaw.(system.TransferPrepareView)
	up := httptest.NewRequest(http.MethodPut, imgPrep.UploadURL, bytes.NewReader(imgPayload))
	up.Header.Set("Authorization", "Bearer "+imgPrep.Token)
	upResp := httptest.NewRecorder()
	router.ServeHTTP(upResp, up)
	if envelopeCode(upResp.Body.Bytes()) != 0 {
		t.Fatalf("image upload failed: %s", upResp.Body.String())
	}
	imgRow, err := attachments.GetByID(imgPrep.Attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if imgRow.GetThumbStatus() != status.AttachmentThumbPending {
		t.Fatalf("thumb_status = %q want pending", imgRow.GetThumbStatus())
	}
	if _, err := svc.DownloadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"id":%q,"variant":"thumb"}`, imgPrep.Attachment.ID,
	))); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("pending thumb should be unavailable, err=%v", err)
	}

	rc, _, err := application.Storage.Open(ctxA, imgRow.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	thumbBytes, err := attachment.GenerateImageThumb(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	thumbKey, err := storage.BuildKey(storage.KeyParts{TenantID: "tenant-a", Namespace: "attachments"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := application.Storage.Put(ctxA, storage.PutRequest{
		Key: thumbKey, Body: bytes.NewReader(thumbBytes), Size: int64(len(thumbBytes)), ContentType: "image/jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attachments.Where("id = ?", imgRow.ID).Updates(map[string]any{
		"thumb_status": status.AttachmentThumbReady, "thumb_object_key": info.Key,
		"thumb_content_type": "image/jpeg", "thumb_size": info.Size,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := svc.DownloadPrepare(ctxB, json.RawMessage(fmt.Sprintf(
		`{"id":%q,"variant":"thumb"}`, imgPrep.Attachment.ID,
	))); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("cross-tenant thumb prepare err=%v", err)
	}

	thumbPrepRaw, err := svc.DownloadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"id":%q,"variant":"thumb"}`, imgPrep.Attachment.ID,
	)))
	if err != nil {
		t.Fatal(err)
	}
	thumbPrep := thumbPrepRaw.(system.TransferPrepareView)
	getReq := httptest.NewRequest(http.MethodGet, thumbPrep.DownloadURL, nil)
	getReq.Header.Set("Authorization", "Bearer "+thumbPrep.Token)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	body, _ := io.ReadAll(getResp.Body)
	if getResp.Code != http.StatusOK || len(body) < 50 {
		t.Fatalf("thumb download status=%d len=%d body=%s", getResp.Code, len(body), getResp.Body.String())
	}
	if ct := getResp.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("content-type=%q", ct)
	}

	// Compensation requeues pending and explicitly retryable failures only.
	retryable, permanent := true, false
	for _, candidate := range []model.Attachment{
		{ID: "thumb-pending", TenantID: "tenant-a", Provider: "local", ObjectKey: "pending", OriginalName: "pending.png", ContentType: "image/png", Status: status.AttachmentReady, UploaderID: "user-a", ThumbStatus: thumbStatusPtr(status.AttachmentThumbPending)},
		{ID: "thumb-retryable", TenantID: "tenant-a", Provider: "local", ObjectKey: "retryable", OriginalName: "retryable.png", ContentType: "image/png", Status: status.AttachmentReady, UploaderID: "user-a", ThumbStatus: thumbStatusPtr(status.AttachmentThumbFailed), ThumbRetryable: &retryable},
		{ID: "thumb-permanent", TenantID: "tenant-a", Provider: "local", ObjectKey: "permanent", OriginalName: "permanent.png", ContentType: "image/png", Status: status.AttachmentReady, UploaderID: "user-a", ThumbStatus: thumbStatusPtr(status.AttachmentThumbFailed), ThumbRetryable: &permanent},
		{ID: "thumb-historical", TenantID: "tenant-a", Provider: "local", ObjectKey: "historical", OriginalName: "historical.png", ContentType: "image/png", Status: status.AttachmentReady, UploaderID: "user-a", ThumbStatus: thumbStatusPtr(status.AttachmentThumbFailed)},
	} {
		if err := attachments.Create(&candidate); err != nil {
			t.Fatal(err)
		}
	}
	enqueuer := &recordingJobEnqueuer{}
	application.Jobs = &job.Runtime{Enabled: true, Enqueuer: enqueuer}
	count, err := systemjobs.RequeuePendingThumbs(ctxA, application, 20)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(enqueuer.inputs) != 2 {
		t.Fatalf("requeued=%d inputs=%d", count, len(enqueuer.inputs))
	}
	requeued := map[string]bool{}
	for _, input := range enqueuer.inputs {
		payload := input.Payload.(systemjobs.GenerateThumbPayload)
		requeued[payload.AttachmentID] = true
	}
	if !requeued["thumb-pending"] || !requeued["thumb-retryable"] || requeued["thumb-permanent"] || requeued["thumb-historical"] {
		t.Fatalf("unexpected requeue set: %#v", requeued)
	}

	// Delete cascades thumb object.
	if _, err := svc.Delete(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q}`, imgPrep.Attachment.ID))); err != nil {
		t.Fatal(err)
	}
	if ok, _ := application.Storage.Exists(ctxA, info.Key); ok {
		t.Fatal("expected thumb object deleted")
	}
}

func thumbStatusPtr(value status.AttachmentThumbStatus) *status.AttachmentThumbStatus { return &value }

func mustPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 200, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
