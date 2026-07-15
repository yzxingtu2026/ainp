package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestAttachmentUploadBindDownloadRangeAndIsolation(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_attach_" + uuid.NewString()[:8]
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
	for _, row := range []tenant.Tenant{
		{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled},
		{ID: "tenant-b", Name: "B", Status: status.ActivationEnabled},
	} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []model.User{
		{ID: "user-a", TenantID: "tenant-a", Username: "admin-a", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "user-a2", TenantID: "tenant-a", Username: "peer-a", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "user-b", TenantID: "tenant-b", Username: "admin-b", PasswordHash: "x", Status: status.ActivationEnabled},
	} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	denied := 0
	if err := application.AttachmentPolicies.Register(attachment.FuncPolicy{
		Type: "demo.task",
		Fn: func(_ context.Context, _, userID, resourceID, _ string) error {
			if resourceID == "task-deny" {
				denied++
				return fmt.Errorf("denied")
			}
			if userID == "" {
				return fmt.Errorf("missing user")
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewAttachmentService(testDB, application.Storage, application.TransferTokens, application.AttachmentPolicies, nil)

	perms := []string{
		system.ActionAttachmentUploadPrepare, system.ActionAttachmentDownloadPrepare,
		system.ActionAttachmentBind, system.ActionAttachmentUnbind,
		system.ActionAttachmentListByResource, system.ActionAttachmentDelete,
	}
	ctxA := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})
	ctxA2 := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", UserID: "user-a2", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})
	ctxB := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-b", UserID: "user-b", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})

	payload := []byte("hello-attachment-range-0123456789")
	prepared, err := svc.UploadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"original_name":"note.txt","content_type":"text/plain","size":%d}`, len(payload),
	)))
	if err != nil {
		t.Fatal(err)
	}
	prep := prepared.(system.TransferPrepareView)
	if prep.Attachment.Status != string(status.AttachmentPending) || prep.Token == "" || prep.UploadURL == "" {
		t.Fatalf("prepare = %#v", prep)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	system.RegisterAttachmentTransferRoutes(router, svc)

	uploadReq := httptest.NewRequest(http.MethodPut, prep.UploadURL, bytes.NewReader(payload))
	uploadReq.Header.Set("Authorization", "Bearer "+prep.Token)
	uploadReq.Header.Set("Content-Type", "text/plain")
	uploadResp := httptest.NewRecorder()
	router.ServeHTTP(uploadResp, uploadReq)
	if code := envelopeCode(uploadResp.Body.Bytes()); code != 0 {
		t.Fatalf("upload status=%d code=%d body=%s", uploadResp.Code, code, uploadResp.Body.String())
	}

	// upload token one-shot / not pending after success
	replayReq := httptest.NewRequest(http.MethodPut, prep.UploadURL, bytes.NewReader(payload))
	replayReq.Header.Set("Authorization", "Bearer "+prep.Token)
	replayResp := httptest.NewRecorder()
	router.ServeHTTP(replayResp, replayReq)
	if code := envelopeCode(replayResp.Body.Bytes()); code == 0 {
		t.Fatalf("expected upload token replay rejection, body=%s", replayResp.Body.String())
	}

	if _, err := svc.Bind(ctxA, json.RawMessage(fmt.Sprintf(
		`{"attachment_id":%q,"resource_type":"unknown.type","resource_id":"x","field":"file"}`, prep.Attachment.ID,
	))); appErrorCode(err) != errs.CodeForbidden {
		t.Fatalf("unregistered resource bind err=%v", err)
	}
	if _, err := svc.Bind(ctxA, json.RawMessage(fmt.Sprintf(
		`{"attachment_id":%q,"resource_type":"demo.task","resource_id":"task-deny","field":"file"}`, prep.Attachment.ID,
	))); appErrorCode(err) != errs.CodeForbidden {
		t.Fatalf("policy deny bind err=%v", err)
	}
	if denied == 0 {
		t.Fatal("expected access policy invoked")
	}
	if _, err := svc.Bind(ctxA, json.RawMessage(fmt.Sprintf(
		`{"attachment_id":%q,"resource_type":"demo.task","resource_id":"task-1","field":"file"}`, prep.Attachment.ID,
	))); err != nil {
		t.Fatal(err)
	}

	listed, err := svc.ListByResource(ctxA, json.RawMessage(`{"resource_type":"demo.task","resource_id":"task-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	items := listed.(map[string]any)["items"].([]system.AttachmentView)
	if len(items) != 1 || items[0].ID != prep.Attachment.ID || items[0].Status != string(status.AttachmentReady) {
		t.Fatalf("list = %#v", listed)
	}

	if _, err := svc.DownloadPrepare(ctxB, json.RawMessage(fmt.Sprintf(`{"id":%q}`, prep.Attachment.ID))); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("cross-tenant download prepare err=%v", err)
	}

	downPrepRaw, err := svc.DownloadPrepare(ctxA2, json.RawMessage(fmt.Sprintf(`{"id":%q}`, prep.Attachment.ID)))
	if err != nil {
		t.Fatal(err)
	}
	downPrep := downPrepRaw.(system.TransferPrepareView)

	headReq := httptest.NewRequest(http.MethodHead, downPrep.DownloadURL+"?token="+downPrep.Token, nil)
	headResp := httptest.NewRecorder()
	router.ServeHTTP(headResp, headReq)
	if headResp.Code != http.StatusOK || headResp.Header().Get("Content-Length") != fmt.Sprintf("%d", len(payload)) {
		t.Fatalf("head status=%d len=%s", headResp.Code, headResp.Header().Get("Content-Length"))
	}
	if !strings.Contains(headResp.Header().Get("Content-Disposition"), "note.txt") {
		t.Fatalf("disposition=%q", headResp.Header().Get("Content-Disposition"))
	}

	rangeReq := httptest.NewRequest(http.MethodGet, downPrep.DownloadURL, nil)
	rangeReq.Header.Set("X-Attachment-Token", downPrep.Token)
	rangeReq.Header.Set("Range", "bytes=6-15")
	rangeResp := httptest.NewRecorder()
	router.ServeHTTP(rangeResp, rangeReq)
	if rangeResp.Code != http.StatusPartialContent {
		t.Fatalf("range status=%d body=%s", rangeResp.Code, rangeResp.Body.String())
	}
	got := rangeResp.Body.Bytes()
	if string(got) != string(payload[6:16]) {
		t.Fatalf("range body=%q want=%q", got, payload[6:16])
	}
	if !strings.HasPrefix(rangeResp.Header().Get("Content-Range"), "bytes 6-15/") {
		t.Fatalf("content-range=%q", rangeResp.Header().Get("Content-Range"))
	}

	fullReq := httptest.NewRequest(http.MethodGet, downPrep.DownloadURL, nil)
	fullReq.Header.Set("Authorization", "Bearer "+downPrep.Token)
	fullResp := httptest.NewRecorder()
	router.ServeHTTP(fullResp, fullReq)
	body, _ := io.ReadAll(fullResp.Body)
	if fullResp.Code != http.StatusOK || !bytes.Equal(body, payload) {
		t.Fatalf("full download status=%d body=%q", fullResp.Code, body)
	}

	if _, err := svc.Delete(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q}`, prep.Attachment.ID))); err == nil {
		t.Fatal("expected delete blocked while linked")
	}
	if _, err := svc.Unbind(ctxA, json.RawMessage(fmt.Sprintf(
		`{"attachment_id":%q,"resource_type":"demo.task","resource_id":"task-1","field":"file"}`, prep.Attachment.ID,
	))); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q}`, prep.Attachment.ID))); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DownloadPrepare(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q}`, prep.Attachment.ID))); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("deleted download prepare err=%v", err)
	}
}

func TestAttachmentExpiredPendingCleanup(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_attach_clean_" + uuid.NewString()[:8]
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
	if err := testDB.Create(&model.User{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}

	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewAttachmentService(testDB, application.Storage, application.TransferTokens, application.AttachmentPolicies, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true,
		Permissions: []string{system.ActionAttachmentUploadPrepare},
	})
	prepared, err := svc.UploadPrepare(ctx, json.RawMessage(`{"original_name":"old.txt","content_type":"text/plain","size":5}`))
	if err != nil {
		t.Fatal(err)
	}
	id := prepared.(system.TransferPrepareView).Attachment.ID
	past := time.Now().UTC().Add(-time.Hour)
	if err := testDB.Model(&model.Attachment{}).Where("id = ?", id).Update("expires_at", past).Error; err != nil {
		t.Fatal(err)
	}
	cleaned, err := svc.CleanupExpired(ctx, 10)
	if err != nil || cleaned != 1 {
		t.Fatalf("cleanup = %d %v", cleaned, err)
	}
	var count int64
	if err := testDB.Unscoped().Model(&model.Attachment{}).Where("id = ? AND deleted_at IS NOT NULL", id).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("soft-deleted count=%d err=%v", count, err)
	}
}

func envelopeCode(body []byte) int {
	var env struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(body, &env)
	return env.Code
}
