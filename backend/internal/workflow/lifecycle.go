package workflow

import (
	"errors"
	"fmt"
	"time"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrWorkflowJobActive is returned when a workflow already has a non-terminal Job.
var ErrWorkflowJobActive = errors.New("workflow already has an active job")

// ProcessInterruptedReason is the durable marker written when a leftover
// running/finalizing Job is settled on process restart (#38).
const ProcessInterruptedReason = "process_interrupted"

// ClaimActiveJob creates a new running Job for the workflow only when no other
// active Job exists. A partial unique index is the actual arbiter so manual,
// cron, and compensation races cannot both win.
func ClaimActiveJob(db *gorm.DB, workflowID uint, triggeredBy string) (*models.Job, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	start := time.Now()
	job := models.Job{
		WorkflowID:  workflowID,
		Status:      models.JobStatusRunning,
		StartTime:   &start,
		TriggeredBy: triggeredBy,
	}
	err := db.Create(&job).Error
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return nil, ErrWorkflowJobActive
		}
		return nil, err
	}
	return &job, nil
}

// SettleInterruptedJobs marks leftover running/finalizing Jobs and their
// non-terminal Executions as cancelled / process_interrupted. It must run on
// process start before any compensation scheduling (#38).
func SettleInterruptedJobs(db *gorm.DB) (jobsSettled, executionsSettled int, err error) {
	if db == nil {
		return 0, 0, fmt.Errorf("database is nil")
	}
	now := time.Now()
	err = db.Transaction(func(tx *gorm.DB) error {
		var leftover []models.Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status IN ?", models.ActiveJobStatuses).
			Find(&leftover).Error; err != nil {
			return err
		}
		for i := range leftover {
			job := &leftover[i]
			res := tx.Model(&models.JobExecution{}).
				Where("job_id = ? AND status IN ?", job.ID, []models.ExecutionStatus{
					models.ExecutionStatusPending,
					models.ExecutionStatusRunning,
				}).
				Updates(map[string]interface{}{
					"status":        models.ExecutionStatusFailed,
					"error_message": ProcessInterruptedReason,
				})
			if res.Error != nil {
				return res.Error
			}
			executionsSettled += int(res.RowsAffected)

			job.Status = models.JobStatusCancelled
			job.EndTime = &now
			if job.ErrorCount == 0 {
				job.ErrorCount = 1
			}
			if err := tx.Save(job).Error; err != nil {
				return err
			}
			jobsSettled++
			logger.Infof("🧹 收口遗留 Job [JobID=%d, WorkflowID=%d] → cancelled/%s",
				job.ID, job.WorkflowID, ProcessInterruptedReason)
		}
		return nil
	})
	return jobsSettled, executionsSettled, err
}

// MarkJobFinalizing transitions a running Job to finalizing so the execution
// lock remains held through report persistence.
func MarkJobFinalizing(db *gorm.DB, job *models.Job) error {
	if db == nil || job == nil {
		return fmt.Errorf("job or database is nil")
	}
	job.Status = models.JobStatusFinalizing
	return db.Model(job).Update("status", models.JobStatusFinalizing).Error
}

// HasReportForJob reports whether a report row already exists for the job
// (unique JobID). Used for idempotent report generation.
func HasReportForJob(db *gorm.DB, jobID uint) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("database is nil")
	}
	var count int64
	if err := db.Model(&models.Report{}).Where("job_id = ?", jobID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ErrCompensationNotAllowed is returned when a Job cannot start "retry failed only".
var ErrCompensationNotAllowed = errors.New("compensation not allowed for this job")

// ErrCompensationAlreadyExists is returned when the original Job already has a compensation Job.
var ErrCompensationAlreadyExists = errors.New("compensation job already exists")

// FailedPodcastIDsFromJob returns podcast IDs whose final execution on the job failed.
func FailedPodcastIDsFromJob(db *gorm.DB, jobID uint) ([]uint, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	var executions []models.JobExecution
	if err := db.Where("job_id = ? AND status = ?", jobID, models.ExecutionStatusFailed).Find(&executions).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(executions))
	seen := map[uint]struct{}{}
	for _, ex := range executions {
		if ex.PodcastID == nil || *ex.PodcastID == 0 {
			continue
		}
		if _, ok := seen[*ex.PodcastID]; ok {
			continue
		}
		seen[*ex.PodcastID] = struct{}{}
		ids = append(ids, *ex.PodcastID)
	}
	return ids, nil
}

// ValidateCompensationSource checks that a Job is partial, terminal, and not yet compensated.
func ValidateCompensationSource(job *models.Job) error {
	if job == nil {
		return fmt.Errorf("%w: job is nil", ErrCompensationNotAllowed)
	}
	if job.Status != models.JobStatusPartial {
		return fmt.Errorf("%w: status=%s (only partial)", ErrCompensationNotAllowed, job.Status)
	}
	if job.CompensatedByJobID != nil && *job.CompensatedByJobID > 0 {
		return ErrCompensationAlreadyExists
	}
	return nil
}
