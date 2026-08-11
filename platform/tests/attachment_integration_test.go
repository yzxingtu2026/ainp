package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync"
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
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
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
	for _, row := range []model.TenantAccount{
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
		TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})
	ctxA2 := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", PrincipalID: "user-a2", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})
	ctxB := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-b", PrincipalID: "user-b", Realm: auth.RealmTenant, Authenticated: true, Permissions: perms,
	})

	payload := []byte("hello-attachment-range-0123456789")
	prepared, err := svc.UploadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"original_name":"note.txt","content_type":"text/plain","size":%d}`, len(payload),
	)))
	if err != nil {
		t.Fatal(err)
	}
	prep := prepared.(system.TransferPrepareView)
	if prep.Attachment.Status != string(status.AttachmentPending) || prep.Token == "" || prep.UploadURL == "" || prep.Method != http.MethodPut || prep.Transport != attachment.TransferTransportRaw || prep.FieldName != "" {
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
	adminList, err := svc.List(ctxA, json.RawMessage(`{"scope":"mine","page":1,"page_size":20}`))
	if err != nil {
		t.Fatal(err)
	}
	adminPage := adminList.(pagination.Result[system.AttachmentView])
	if len(adminPage.Items) != 1 || adminPage.Items[0].UploaderName != "admin-a" {
		t.Fatalf("attachment uploader names = %#v", adminPage.Items)
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

	multipartPayload := []byte("multipart-content-from-mini-program")
	multipartPrepared, err := svc.UploadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"original_name":"mini.txt","content_type":"text/plain","size":%d,"transport":"multipart"}`, len(multipartPayload),
	)))
	if err != nil {
		t.Fatal(err)
	}
	multipartPrep := multipartPrepared.(system.TransferPrepareView)
	if multipartPrep.Method != http.MethodPost || multipartPrep.Transport != attachment.TransferTransportMultipart || multipartPrep.FieldName != "file" {
		t.Fatalf("multipart prepare = %#v", multipartPrep)
	}
	wrongMethodReq := httptest.NewRequest(http.MethodPut, multipartPrep.UploadURL, bytes.NewReader(multipartPayload))
	wrongMethodReq.Header.Set("Authorization", "Bearer "+multipartPrep.Token)
	wrongMethodResp := httptest.NewRecorder()
	router.ServeHTTP(wrongMethodResp, wrongMethodReq)
	if code := envelopeCode(wrongMethodResp.Body.Bytes()); code == 0 {
		t.Fatalf("multipart token must reject raw PUT: %s", wrongMethodResp.Body.String())
	}
	multipartBody, multipartType := multipartFileBody(t, multipartPrep.FieldName, "untrusted-name.exe", "application/x-untrusted", multipartPayload)
	multipartReq := httptest.NewRequest(http.MethodPost, multipartPrep.UploadURL, bytes.NewReader(multipartBody))
	multipartReq.Header.Set("X-Attachment-Token", multipartPrep.Token)
	multipartReq.Header.Set("Content-Type", multipartType)
	multipartResp := httptest.NewRecorder()
	router.ServeHTTP(multipartResp, multipartReq)
	if code := envelopeCode(multipartResp.Body.Bytes()); code != 0 {
		t.Fatalf("multipart upload status=%d code=%d body=%s", multipartResp.Code, code, multipartResp.Body.String())
	}
	var multipartEnvelope struct {
		Data struct {
			Attachment system.AttachmentView `json:"attachment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(multipartResp.Body.Bytes(), &multipartEnvelope); err != nil {
		t.Fatal(err)
	}
	if multipartEnvelope.Data.Attachment.OriginalName != "mini.txt" || multipartEnvelope.Data.Attachment.ContentType != "text/plain" {
		t.Fatalf("multipart metadata was not preserved: %#v", multipartEnvelope.Data.Attachment)
	}
	replayBody, replayType := multipartFileBody(t, multipartPrep.FieldName, "again.txt", "text/plain", multipartPayload)
	replayMultipartReq := httptest.NewRequest(http.MethodPost, multipartPrep.UploadURL, bytes.NewReader(replayBody))
	replayMultipartReq.Header.Set("Authorization", "Bearer "+multipartPrep.Token)
	replayMultipartReq.Header.Set("Content-Type", replayType)
	replayMultipartResp := httptest.NewRecorder()
	router.ServeHTTP(replayMultipartResp, replayMultipartReq)
	if code := envelopeCode(replayMultipartResp.Body.Bytes()); code == 0 {
		t.Fatalf("expected multipart token replay rejection: %s", replayMultipartResp.Body.String())
	}

	rawMismatchPrepared, err := svc.UploadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"original_name":"raw-mismatch.txt","content_type":"text/plain","size":%d}`, len(multipartPayload),
	)))
	if err != nil {
		t.Fatal(err)
	}
	rawMismatchPrep := rawMismatchPrepared.(system.TransferPrepareView)
	rawAsMultipartBody, rawAsMultipartType := multipartFileBody(t, "file", "raw.txt", "text/plain", multipartPayload)
	rawAsMultipartReq := httptest.NewRequest(http.MethodPost, rawMismatchPrep.UploadURL, bytes.NewReader(rawAsMultipartBody))
	rawAsMultipartReq.Header.Set("Authorization", "Bearer "+rawMismatchPrep.Token)
	rawAsMultipartReq.Header.Set("Content-Type", rawAsMultipartType)
	rawAsMultipartResp := httptest.NewRecorder()
	router.ServeHTTP(rawAsMultipartResp, rawAsMultipartReq)
	if code := envelopeCode(rawAsMultipartResp.Body.Bytes()); code == 0 {
		t.Fatalf("raw token must reject multipart POST: %s", rawAsMultipartResp.Body.String())
	}

	concurrentPrepared, err := svc.UploadPrepare(ctxA, json.RawMessage(fmt.Sprintf(
		`{"original_name":"concurrent.txt","content_type":"text/plain","size":%d,"transport":"multipart"}`, len(multipartPayload),
	)))
	if err != nil {
		t.Fatal(err)
	}
	concurrentPrep := concurrentPrepared.(system.TransferPrepareView)
	start := make(chan struct{})
	responses := make([]*httptest.ResponseRecorder, 2)
	requestBodies := make([][]byte, len(responses))
	contentTypes := make([]string, len(responses))
	for i := range responses {
		requestBodies[i], contentTypes[i] = multipartFileBody(t, "file", "concurrent.txt", "text/plain", multipartPayload)
	}
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, concurrentPrep.UploadURL, bytes.NewReader(requestBodies[index]))
			req.Header.Set("Authorization", "Bearer "+concurrentPrep.Token)
			req.Header.Set("Content-Type", contentTypes[index])
			responses[index] = httptest.NewRecorder()
			<-start
			router.ServeHTTP(responses[index], req)
		}(i)
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, response := range responses {
		if envelopeCode(response.Body.Bytes()) == 0 {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent multipart successes=%d bodies=%q / %q", successes, responses[0].Body.String(), responses[1].Body.String())
	}
}

func TestAttachmentMultipartTransferContract(t *testing.T) {
	svc, router, ctx, tokens := newAttachmentTransferHarness(t)
	payload := []byte("x")
	prepare := func(name string, size int64) system.TransferPrepareView {
		t.Helper()
		prepared, err := svc.UploadPrepare(ctx, json.RawMessage(fmt.Sprintf(
			`{"original_name":%q,"content_type":"text/plain","size":%d,"transport":"multipart"}`, name, size,
		)))
		if err != nil {
			t.Fatal(err)
		}
		return prepared.(system.TransferPrepareView)
	}
	upload := func(prep system.TransferPrepareView, body io.Reader, contentType string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, prep.UploadURL, body)
		req.Header.Set("Authorization", "Bearer "+prep.Token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	for _, test := range []struct {
		name         string
		expectedSize int64
		body         func(*testing.T) ([]byte, string)
		messageKey   string
		consumes     bool
	}{
		{
			name:         "missing file",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(*multipart.Writer) {})
			},
			messageKey: "attachment.multipart_invalid", consumes: true,
		},
		{
			name:         "ordinary field",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(writer *multipart.Writer) {
					if err := writer.WriteField("file", "not-a-file"); err != nil {
						t.Fatal(err)
					}
				})
			},
			messageKey: "attachment.multipart_invalid", consumes: true,
		},
		{
			name:         "wrong file field",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartFileBody(t, "upload", "ignored.txt", "text/plain", payload)
			},
			messageKey: "attachment.multipart_invalid", consumes: true,
		},
		{
			name:         "duplicate file",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(writer *multipart.Writer) {
					writeMultipartFilePart(t, writer, "file", "first.txt", "text/plain", payload)
					writeMultipartFilePart(t, writer, "file", "second.txt", "text/plain", payload)
				})
			},
			messageKey: "attachment.multipart_invalid", consumes: true,
		},
		{
			name:         "file plus field",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(writer *multipart.Writer) {
					writeMultipartFilePart(t, writer, "file", "first.txt", "text/plain", payload)
					if err := writer.WriteField("tenant_id", "untrusted"); err != nil {
						t.Fatal(err)
					}
				})
			},
			messageKey: "attachment.multipart_invalid", consumes: true,
		},
		{
			name:         "nested multipart",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(writer *multipart.Writer) {
					header := make(textproto.MIMEHeader)
					header.Set("Content-Disposition", `form-data; name="file"; filename="ignored.bin"`)
					header.Set("Content-Type", `multipart/mixed; boundary=nested`)
					part, err := writer.CreatePart(header)
					if err != nil {
						t.Fatal(err)
					}
					_, _ = part.Write(payload)
				})
			},
			messageKey: "attachment.multipart_invalid", consumes: true,
		},
		{
			name:         "content transfer encoding",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(writer *multipart.Writer) {
					header := make(textproto.MIMEHeader)
					header.Set("Content-Disposition", `form-data; name="file"; filename="ignored.txt"`)
					header.Set("Content-Type", "text/plain")
					header.Set("Content-Transfer-Encoding", "quoted-printable")
					part, err := writer.CreatePart(header)
					if err != nil {
						t.Fatal(err)
					}
					_, _ = part.Write(payload)
				})
			},
			messageKey: "attachment.multipart_invalid", consumes: true,
		},
		{
			name:         "oversized part header",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(writer *multipart.Writer) {
					header := make(textproto.MIMEHeader)
					header.Set("Content-Disposition", `form-data; name="file"; filename="ignored.txt"`)
					header.Set("Content-Type", "text/plain")
					header.Set("X-Fill", strings.Repeat("a", 17<<10))
					part, err := writer.CreatePart(header)
					if err != nil {
						t.Fatal(err)
					}
					_, _ = part.Write(payload)
				})
			},
			messageKey: "attachment.multipart_limit_exceeded", consumes: true,
		},
		{
			name:         "oversized metadata",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartCustomBody(t, func(writer *multipart.Writer) {
					header := make(textproto.MIMEHeader)
					header.Set("Content-Disposition", `form-data; name="file"; filename="ignored.txt"`)
					header.Set("Content-Type", "text/plain")
					header.Set("X-Fill", strings.Repeat("a", 40<<10))
					part, err := writer.CreatePart(header)
					if err != nil {
						t.Fatal(err)
					}
					_, _ = part.Write(payload)
				})
			},
			messageKey: "attachment.multipart_limit_exceeded", consumes: true,
		},
		{
			name:         "truncated file",
			expectedSize: 2,
			body: func(t *testing.T) ([]byte, string) {
				return multipartFileBody(t, "file", "short.txt", "text/plain", payload)
			},
			messageKey: "attachment.size_mismatch", consumes: true,
		},
		{
			name:         "oversized file",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartFileBody(t, "file", "long.txt", "text/plain", []byte("xx"))
			},
			messageKey: "attachment.size_mismatch", consumes: true,
		},
		{
			name:         "zero byte file",
			expectedSize: 1,
			body: func(t *testing.T) ([]byte, string) {
				return multipartFileBody(t, "file", "empty.txt", "text/plain", nil)
			},
			messageKey: "attachment.size_mismatch", consumes: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			prep := prepare(strings.ReplaceAll(test.name, " ", "-")+".txt", test.expectedSize)
			body, contentType := test.body(t)
			resp := upload(prep, bytes.NewReader(body), contentType)
			if key := envelopeMessageKey(resp.Body.Bytes()); key != test.messageKey {
				t.Fatalf("message_key=%q want=%q body=%s", key, test.messageKey, resp.Body.String())
			}
			if envelopeRequestID(resp.Body.Bytes()) == "" {
				t.Fatalf("missing request_id: %s", resp.Body.String())
			}
			if test.consumes {
				retryBody, retryType := multipartFileBody(t, "file", "retry.txt", "text/plain", bytes.Repeat(payload, int(test.expectedSize)))
				retry := upload(prep, bytes.NewReader(retryBody), retryType)
				if envelopeCode(retry.Body.Bytes()) == 0 {
					t.Fatalf("malformed upload token was reusable: %s", retry.Body.String())
				}
			}
		})
	}

	outerPrep := prepare("outer.txt", 1)
	for _, test := range []struct {
		name        string
		contentType string
		body        []byte
		messageKey  string
	}{
		{name: "wrong content type", contentType: "application/json", body: []byte("{}"), messageKey: "attachment.multipart_invalid"},
		{name: "missing boundary", contentType: "multipart/form-data", body: payload, messageKey: "attachment.multipart_invalid"},
		{name: "invalid boundary", contentType: `multipart/form-data; boundary="bad@boundary"`, body: payload, messageKey: "attachment.multipart_invalid"},
		{name: "long boundary", contentType: "multipart/form-data; boundary=" + strings.Repeat("a", 71), body: payload, messageKey: "attachment.multipart_invalid"},
		{name: "overall body limit", contentType: "multipart/form-data; boundary=valid", body: bytes.Repeat(payload, (64<<10)+2), messageKey: "attachment.multipart_limit_exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp := upload(outerPrep, bytes.NewReader(test.body), test.contentType)
			if key := envelopeMessageKey(resp.Body.Bytes()); key != test.messageKey {
				t.Fatalf("message_key=%q want=%q body=%s", key, test.messageKey, resp.Body.String())
			}
		})
	}

	identityPrep := prepare("identity.txt", 1)
	validBody, validType := multipartFileBody(t, "file", "identity.txt", "text/plain", payload)
	wrongIDReq := httptest.NewRequest(http.MethodPost, "/api/v1/attachments/transfer/other-attachment", bytes.NewReader(validBody))
	wrongIDReq.Header.Set("Authorization", "Bearer "+identityPrep.Token)
	wrongIDReq.Header.Set("Content-Type", validType)
	wrongIDResp := httptest.NewRecorder()
	router.ServeHTTP(wrongIDResp, wrongIDReq)
	if envelopeCode(wrongIDResp.Body.Bytes()) == 0 {
		t.Fatalf("attachment id mismatch was accepted: %s", wrongIDResp.Body.String())
	}
	forgedInputs := []attachment.IssueTransferInput{
		{TenantID: "tenant-b", PrincipalID: "user-a", AttachmentID: identityPrep.Attachment.ID, Op: attachment.OpUpload, Transport: attachment.TransferTransportMultipart, Size: 1, TTL: time.Minute},
		{TenantID: "tenant-a", PrincipalID: "other-user", AttachmentID: identityPrep.Attachment.ID, Op: attachment.OpUpload, Transport: attachment.TransferTransportMultipart, Size: 1, TTL: time.Minute},
	}
	for _, input := range forgedInputs {
		forgedToken, _, _, err := tokens.Issue(input)
		if err != nil {
			t.Fatal(err)
		}
		body, contentType := multipartFileBody(t, "file", "identity.txt", "text/plain", payload)
		req := httptest.NewRequest(http.MethodPost, identityPrep.UploadURL, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+forgedToken)
		req.Header.Set("Content-Type", contentType)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if envelopeCode(resp.Body.Bytes()) == 0 {
			t.Fatalf("forged transfer identity was accepted: %s", resp.Body.String())
		}
	}

	for _, test := range []struct {
		name       string
		reader     func([]byte, context.CancelFunc) io.Reader
		messageKey string
	}{
		{
			name: "context cancelled",
			reader: func(body []byte, cancel context.CancelFunc) io.Reader {
				return &cancelAfterReader{r: bytes.NewReader(body), remaining: 1024, cancel: cancel}
			},
			messageKey: "attachment.upload_cancelled",
		},
		{
			name: "body read failure",
			reader: func(body []byte, _ context.CancelFunc) io.Reader {
				return &failAfterReader{r: bytes.NewReader(body), remaining: 1024, err: errors.New("synthetic read failure")}
			},
			messageKey: "attachment.multipart_invalid",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			streamPayload := bytes.Repeat([]byte("a"), 64<<10)
			prep := prepare(strings.ReplaceAll(test.name, " ", "-")+".txt", int64(len(streamPayload)))
			body, contentType := multipartFileBody(t, "file", "stream.txt", "text/plain", streamPayload)
			requestCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, prep.UploadURL, test.reader(body, cancel)).WithContext(requestCtx)
			req.Header.Set("Authorization", "Bearer "+prep.Token)
			req.Header.Set("Content-Type", contentType)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if key := envelopeMessageKey(resp.Body.Bytes()); key != test.messageKey {
				t.Fatalf("message_key=%q want=%q body=%s", key, test.messageKey, resp.Body.String())
			}
			retryBody, retryType := multipartFileBody(t, "file", "retry.txt", "text/plain", streamPayload)
			retry := upload(prep, bytes.NewReader(retryBody), retryType)
			if envelopeCode(retry.Body.Bytes()) == 0 {
				t.Fatalf("failed stream token was reusable: %s", retry.Body.String())
			}
		})
	}

	const largeSize = int64(2 << 20)
	largePrep := prepare("streamed-large.txt", largeSize)
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()
	done := make(chan error, 1)
	go func() {
		part, err := writer.CreateFormFile("file", "ignored-client-name.bin")
		if err == nil {
			_, err = io.CopyN(part, repeatedByteReader('a'), largeSize)
		}
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		_ = pipeWriter.CloseWithError(err)
		done <- err
	}()
	streamedResp := upload(largePrep, pipeReader, contentType)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if code := envelopeCode(streamedResp.Body.Bytes()); code != 0 {
		t.Fatalf("streamed multipart upload failed: %s", streamedResp.Body.String())
	}
}

func newAttachmentTransferHarness(t *testing.T) (*system.AttachmentService, *gin.Engine, context.Context, *attachment.TransferTokenService) {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_attach_transfer_" + uuid.NewString()[:8]
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
	if err := testDB.Create(&model.TenantAccount{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewAttachmentService(testDB, application.Storage, application.TransferTokens, application.AttachmentPolicies, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true,
		Permissions: []string{system.ActionAttachmentUploadPrepare},
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("request_id", "issue-250-contract")
		c.Next()
	})
	system.RegisterAttachmentTransferRoutes(router, svc)
	return svc, router, ctx, application.TransferTokens
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
	if err := testDB.Create(&model.TenantAccount{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}

	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewAttachmentService(testDB, application.Storage, application.TransferTokens, application.AttachmentPolicies, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true,
		Permissions: []string{system.ActionAttachmentUploadPrepare},
	})
	prepared, err := svc.UploadPrepare(ctx, json.RawMessage(`{"original_name":"old.txt","content_type":"text/plain","size":5}`))
	if err != nil {
		t.Fatal(err)
	}
	id := prepared.(system.TransferPrepareView).Attachment.ID
	past := time.Now().UTC().Add(-time.Hour)
	tenantDB, err := tenant.TenantDB(ctx, testDB)
	if err != nil {
		t.Fatal(err)
	}
	attachments, _ := tenant.NewTenantRepository[model.Attachment](tenantDB)
	if err := attachments.Where("id = ?", id).Updates(map[string]any{"expires_at": past}).Error; err != nil {
		t.Fatal(err)
	}
	cleaned, err := svc.CleanupExpired(ctx, 10)
	if err != nil || cleaned != 1 {
		t.Fatalf("cleanup = %d %v", cleaned, err)
	}
	platformCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true})
	systemDB, err := tenant.SystemDB(platformCtx, testDB)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := tenant.SystemRaw[struct{ Count int64 }](systemDB, "SELECT COUNT(*) AS count FROM attachments WHERE id = ? AND deleted_at IS NOT NULL", id)
	if err != nil || len(counts) != 1 || counts[0].Count != 1 {
		t.Fatalf("soft-deleted counts=%v err=%v", counts, err)
	}
}

func envelopeCode(body []byte) int {
	var env struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(body, &env)
	return env.Code
}

func envelopeMessageKey(body []byte) string {
	var env struct {
		MessageKey string `json:"message_key"`
	}
	_ = json.Unmarshal(body, &env)
	return env.MessageKey
}

func envelopeRequestID(body []byte) string {
	var env struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(body, &env)
	return env.RequestID
}

func multipartFileBody(t *testing.T, fieldName, filename, contentType string, payload []byte) ([]byte, string) {
	t.Helper()
	return multipartCustomBody(t, func(writer *multipart.Writer) {
		writeMultipartFilePart(t, writer, fieldName, filename, contentType, payload)
	})
}

func multipartCustomBody(t *testing.T, build func(*multipart.Writer)) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	build(writer)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func writeMultipartFilePart(t *testing.T, writer *multipart.Writer, fieldName, filename, contentType string, payload []byte) {
	t.Helper()
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, fieldName, filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
}

type repeatedByteReader byte

func (r repeatedByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

type cancelAfterReader struct {
	r         io.Reader
	remaining int
	cancel    context.CancelFunc
	cancelled bool
}

func (r *cancelAfterReader) Read(p []byte) (int, error) {
	if !r.cancelled && len(p) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	if !r.cancelled {
		r.remaining -= n
		if r.remaining <= 0 {
			r.cancelled = true
			r.cancel()
		}
	}
	return n, err
}

type failAfterReader struct {
	r         io.Reader
	remaining int
	err       error
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, r.err
	}
	if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= n
	return n, err
}
