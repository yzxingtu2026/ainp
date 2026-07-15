package status

// JobRunStatus is the lifecycle of a background job run record.
type JobRunStatus string

const (
	JobPendingEnqueue JobRunStatus = "pending_enqueue"
	JobQueued         JobRunStatus = "queued"
	JobRunning        JobRunStatus = "running"
	JobRetrying       JobRunStatus = "retrying"
	JobSucceeded      JobRunStatus = "succeeded"
	JobFailed         JobRunStatus = "failed"
	JobDead           JobRunStatus = "dead"
	JobCancelled      JobRunStatus = "cancelled"
)

func (s JobRunStatus) Valid() bool {
	switch s {
	case JobPendingEnqueue, JobQueued, JobRunning, JobRetrying, JobSucceeded, JobFailed, JobDead, JobCancelled:
		return true
	default:
		return false
	}
}
