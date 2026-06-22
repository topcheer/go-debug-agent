package debugagent

import (
	"sync"
	"time"
)

// ─── Scheduler registry ─────────────────────────────────────────────────────

// JobRun represents a single execution of a scheduled job.
type JobRun struct {
	Time     string `json:"time"`
	Duration string `json:"duration"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// ScheduledJob represents a recurring or cron-based background job.
type ScheduledJob struct {
	Name      string   `json:"name"`
	Schedule  string   `json:"schedule"`
	NextRun   time.Time `json:"next_run"`
	LastRun   *time.Time `json:"last_run,omitempty"`
	LastError string   `json:"last_error,omitempty"`
	History   []JobRun `json:"history"`

	interval time.Duration
	ticker   *time.Ticker
	done     chan struct{}
}

var (
	scheduledJobs = map[string]*ScheduledJob{}
	schedulerMu   sync.RWMutex
)

// RegisterScheduledJob registers a scheduled job and returns the job struct
// for further configuration. The job will appear in scheduler inspection tools.
func RegisterScheduledJob(name, schedule string) *ScheduledJob {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()
	job := &ScheduledJob{
		Name:     name,
		Schedule: schedule,
		History:  []JobRun{},
	}
	scheduledJobs[name] = job
	return job
}

// StartTicker starts a goroutine that runs fn at the given interval.
// The job is automatically tracked for inspection.
func (j *ScheduledJob) StartTicker(interval time.Duration, fn func() error) {
	schedulerMu.Lock()
	j.interval = interval
	j.ticker = time.NewTicker(interval)
	j.done = make(chan struct{})
	j.NextRun = time.Now().Add(interval)
	schedulerMu.Unlock()

	go func() {
		for {
			select {
			case <-j.ticker.C:
				runScheduledJob(j, fn)
			case <-j.done:
				j.ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops the scheduled job's ticker if running.
func (j *ScheduledJob) Stop() {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()
	if j.done != nil {
		close(j.done)
		j.done = nil
	}
}

// runScheduledJob executes fn and records the run in the job's history.
func runScheduledJob(j *ScheduledJob, fn func() error) {
	defer func() {
		if r := recover(); r != nil {
			schedulerMu.Lock()
			j.LastError = fmtSprintf("panic: %v", r)
			now := time.Now()
			j.LastRun = &now
			j.History = appendHistory(j.History, JobRun{
				Time:     now.Format(time.RFC3339),
				Duration: "0s",
				Success:  false,
				Error:    fmtSprintf("panic: %v", r),
			})
			j.NextRun = time.Now().Add(j.interval)
			schedulerMu.Unlock()
		}
	}()

	start := time.Now()
	err := fn()
	elapsed := time.Since(start)
	now := time.Now()

	schedulerMu.Lock()
	j.LastRun = &now
	if err != nil {
		j.LastError = err.Error()
	} else {
		j.LastError = ""
	}
	run := JobRun{
		Time:     now.Format(time.RFC3339),
		Duration: elapsed.String(),
		Success:  err == nil,
	}
	if err != nil {
		run.Error = err.Error()
	}
	j.History = appendHistory(j.History, run)
	if j.interval > 0 {
		j.NextRun = time.Now().Add(j.interval)
	}
	schedulerMu.Unlock()
}

func appendHistory(history []JobRun, run JobRun) []JobRun {
	history = append(history, run)
	// Keep last 100 entries
	if len(history) > 100 {
		history = history[len(history)-100:]
	}
	return history
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerSchedulerInspector() {
	RegisterTool("get_scheduled_jobs", "List all registered scheduled/cron jobs (name, schedule expression, next run time, last run, last error)", nil, func(args map[string]any) (any, error) {
		schedulerMu.RLock()
		defer schedulerMu.RUnlock()

		if len(scheduledJobs) == 0 {
			return map[string]any{
				"message": "No scheduled jobs registered. Call debugagent.RegisterScheduledJob(name, schedule) to enable scheduler inspection.",
				"count":   0,
			}, nil
		}

		jobs := make([]map[string]any, 0, len(scheduledJobs))
		for name, job := range scheduledJobs {
			entry := map[string]any{
				"name":      name,
				"schedule":  job.Schedule,
				"next_run":  formatTime(job.NextRun),
				"last_run":  formatTimePtr(job.LastRun),
				"last_error": job.LastError,
				"runs":      len(job.History),
			}
			jobs = append(jobs, entry)
		}

		return map[string]any{
			"count": len(scheduledJobs),
			"jobs":  jobs,
		}, nil
	})

	RegisterTool("get_job_history", "Get execution history of a specific scheduled job (run time, duration, success/error)", map[string]ToolParam{
		"job_name": {Type: "string", Description: "Name of the scheduled job", Required: true},
	}, func(args map[string]any) (any, error) {
		schedulerMu.RLock()
		defer schedulerMu.RUnlock()

		jobName, _ := args["job_name"].(string)
		if jobName == "" {
			return nil, fmtErrorf("job_name is required")
		}

		job, ok := scheduledJobs[jobName]
		if !ok {
			available := make([]string, 0, len(scheduledJobs))
			for name := range scheduledJobs {
				available = append(available, name)
			}
			return map[string]any{
				"error":     "Scheduled job not found: " + jobName,
				"available": available,
			}, nil
		}

		result := map[string]any{
			"name":          job.Name,
			"schedule":      job.Schedule,
			"total_runs":    len(job.History),
			"last_error":    job.LastError,
			"history":       job.History,
		}
		if job.LastRun != nil {
			result["last_run"] = job.LastRun.Format(time.RFC3339)
		}

		// Compute success rate
		if len(job.History) > 0 {
			successCount := 0
			for _, run := range job.History {
				if run.Success {
					successCount++
				}
			}
			result["success_rate"] = fmtSprintf("%.1f%%", float64(successCount)/float64(len(job.History))*100)
		}

		return result, nil
	})
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
