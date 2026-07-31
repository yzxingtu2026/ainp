package showcase

import "github.com/yzxingtu2026/ainp/platform/framework/codeset"

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskCompleted TaskStatus = "completed"

	CodeSetTaskStatus = "demo.task_status"
	StateMachineTask  = "demo.task_lifecycle"
	ActionComplete    = "demo.complete"
)

func taskStatusCodeSet() codeset.CodeSet {
	return codeset.CodeSet{
		Key: CodeSetTaskStatus, NameKey: "codeset.demo.task_status",
		Items: []codeset.CodeItem{
			{Value: string(TaskPending), LabelKey: "codeset.demo.task_status.pending", Semantic: codeset.SemanticInfo, Icon: "mdi:clock-outline", Order: 10},
			{Value: string(TaskCompleted), LabelKey: "codeset.demo.task_status.completed", Semantic: codeset.SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 20},
		},
		Order: 100,
	}
}
