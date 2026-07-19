package demo

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const (
	EventNoteCreatedV1 = "demo.note_created.v1"
	ActionNoteCreate   = "demo.note_create"

	HandlerNoteIndex   = "demo.note_index"
	HandlerNoteCounter = "demo.note_counter"
)

// Note is a minimal demo aggregate used to exercise transactional outbox.
type Note struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID  string    `gorm:"size:64;not null;index" json:"tenant_id"`
	Title     string    `gorm:"size:256;not null" json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

func (Note) TableName() string { return "demo_notes" }

// NoteCreatedPayload is the stable DTO for demo.note_created.v1.
type NoteCreatedPayload struct {
	NoteID string `json:"note_id"`
	Title  string `json:"title"`
}

// DemoCounters tracks handler invocations for tests (process-local).
var (
	NoteIndexCalls   atomic.Int64
	NoteCounterCalls atomic.Int64
	noteRegistration struct {
		sync.Once
		err error
	}
)

func registerOutboxDemo(a *app.App) error {
	if a.Outbox == nil || a.DB == nil {
		return nil
	}
	noteRegistration.Do(func() {
		noteRegistration.err = tenantdata.DefaultRegistry.Register(Note{}, tenantdata.ModelOptions{
			Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true,
		})
	})
	if noteRegistration.err != nil {
		return noteRegistration.err
	}
	migrationDB, err := tenantdata.MigrationDB(a.DB)
	if err != nil {
		return err
	}
	if err := migrationDB.AutoMigrate(&Note{}); err != nil {
		return err
	}
	if err := a.Outbox.RegisterDefinition(outbox.Definition{
		EventType:     EventNoteCreatedV1,
		SchemaVersion: 1,
		Description:   "Demo note created",
	}); err != nil {
		return err
	}
	if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{
		HandlerName: HandlerNoteIndex,
		Types:       []string{EventNoteCreatedV1},
		Fn: func(_ context.Context, env outbox.Envelope) error {
			NoteIndexCalls.Add(1)
			_ = env
			return nil
		},
	}); err != nil {
		return err
	}
	if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{
		HandlerName: HandlerNoteCounter,
		Types:       []string{EventNoteCreatedV1},
		Fn: func(_ context.Context, env outbox.Envelope) error {
			NoteCounterCalls.Add(1)
			_ = env
			return nil
		},
	}); err != nil {
		return err
	}

	writer := a.Outbox.Writer
	return a.RegisterAction(action.Action{
		ID: ActionNoteCreate,
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			ac := auth.MustFromContext(ctx)
			if !ac.Authenticated || ac.TenantID == "" {
				return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
			}
			var req struct {
				Title string `json:"title"`
			}
			_ = json.Unmarshal(input, &req)
			if req.Title == "" {
				req.Title = "untitled"
			}
			var note Note
			guarded := tenantdata.GuardedDB(a.DB)
			err := outbox.WithinTx(ctx, guarded.WithContext(ctx), writer, func(tx *gorm.DB, w outbox.Writer) error {
				note = Note{
					ID:        uuid.NewString(),
					Title:     req.Title,
					CreatedAt: time.Now().UTC(),
				}
				if err := tx.Create(&note).Error; err != nil {
					return err
				}
				_, err := w.Append(ctx, tx, outbox.AppendInput{
					EventType:     EventNoteCreatedV1,
					AggregateType: "demo.note",
					AggregateID:   note.ID,
					Payload: NoteCreatedPayload{
						NoteID: note.ID,
						Title:  note.Title,
					},
				})
				return err
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"note": note}, nil
		},
		Meta: action.Metadata{
			Capability:     "demo",
			Name:           "创建演示笔记",
			NameKey:        "action.demo.note_create.name",
			Description:    "同事务写入笔记与 Outbox 事件",
			DescriptionKey: "action.demo.note_create.description",
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/demo/notes",
			},
		},
	})
}
