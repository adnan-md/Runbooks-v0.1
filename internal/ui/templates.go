package ui

import (
        "fmt"
        "html/template"
        "strings"

        "github.com/runbooks/runbooks/internal/store"
)

const tmpl = `
{{define "base"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} - Runbooks</title>
<link rel="stylesheet" href="/static/styles.css">
{{if eq .Active "dashboard"}}<script src="/static/dashboard.js" defer></script>{{end}}
{{if eq .Active "jobs"}}<script src="/static/logs.js" defer></script>{{end}}
</head>
<body>
<div class="layout">
  <aside class="sidebar">
    <h1>Runbooks</h1>
    <nav>
      <a href="/" class="{{if eq .Active "dashboard"}}active{{end}}">Dashboard</a>
      <a href="/jobs" class="{{if eq .Active "jobs"}}active{{end}}">Jobs</a>
      <a href="/workers" class="{{if eq .Active "workers"}}active{{end}}">Workers</a>
      <a href="/schedules" class="{{if eq .Active "schedules"}}active{{end}}">Schedules</a>
    </nav>
  </aside>
  <main class="main">
    {{.Content}}
  </main>
</div>
</body>
</html>
{{end}}
`

func dashboardContent(s stats, jobs []*store.Job, activeWorkers []*store.Worker, runningJobs []*store.Job, failed24h int) string {
        var b strings.Builder
        b.WriteString(`<div class="page-header"><h2>Dashboard</h2>`)
        b.WriteString(`<a href="/jobs/new" class="btn">Submit Job</a></div>`)
        b.WriteString(`<div class="grid grid-4" style="margin-bottom:24px">`)
        statCard(&b, "Total Jobs", s.JobsTotal)
        statCard(&b, "Running", len(runningJobs))
        statCard(&b, "Active Workers", len(activeWorkers))
        statCard(&b, "Failed (24h)", failed24h)
        b.WriteString(`</div>`)

        if len(activeWorkers) > 0 {
                b.WriteString(`<div class="card"><h3 style="margin:0 0 12px 0;font-size:14px;color:#334155">Fleet Status</h3>`)
                b.WriteString(`<table><thead><tr><th>Worker</th><th>Status</th><th>Concurrency</th><th>Last Heartbeat</th></tr></thead><tbody>`)
                for _, w := range activeWorkers {
                        fmt.Fprintf(&b, `<tr><td>%s</td><td><span class="%s">%s</span></td><td>%d</td><td>%s</td></tr>`,
                                w.Name, badgeClass(w.Status), w.Status, w.MaxConcurrency, formatTime(w.LastHeartbeatAt.Time))
                }
                b.WriteString(`</tbody></table>`)
                b.WriteString(`</div>`)
        }

        b.WriteString(`<div class="card"><h3 style="margin:0 0 12px 0;font-size:14px;color:#334155">Recent Jobs</h3>`)
        if len(jobs) == 0 {
                b.WriteString(`<div class="empty">No jobs yet. Submit your first job to get started.</div>`)
        } else {
                b.WriteString(`<table><thead><tr><th>Name</th><th>Command</th><th>Status</th><th>Created</th></tr></thead><tbody>`)
                for _, j := range jobs {
                        display := j.Command
                        if display == "" && j.Script.Valid {
                                display = "[script: " + j.ScriptLanguage.String + "]"
                        }
                        fmt.Fprintf(&b, `<tr><td><a href="/jobs/%s">%s</a></td><td class="mono">%s</td><td><span class="%s">%s</span></td><td>%s</td></tr>`,
                                j.ID, j.Name, display, badgeClass(j.Status), j.Status, formatTime(j.CreatedAt))
                }
                b.WriteString(`</tbody></table>`)
        }
        b.WriteString(`</div>`)
        return b.String()
}

func statCard(b *strings.Builder, label string, value int) {
        fmt.Fprintf(b, `<div class="stat"><div class="stat-label">%s</div><div class="stat-value">%d</div></div>`, label, value)
}

func jobsListContent(jobs []*store.Job, statuses []string, limit, offset, total int) string {
        var b strings.Builder
        b.WriteString(`<div class="page-header"><h2>Jobs</h2><a href="/jobs/new" class="btn">Submit Job</a></div>`)
        b.WriteString(`<div class="card">`)

        b.WriteString(`<div style="margin-bottom:16px;display:flex;gap:8px;flex-wrap:wrap">`)
        b.WriteString(`<a href="/jobs" class="btn btn-secondary btn-sm">All</a>`)
        for _, s := range []string{"QUEUED", "RUNNING", "SUCCEEDED", "FAILED", "RETRYING", "LOST"} {
                active := false
                for _, sel := range statuses {
                        if sel == s {
                                active = true
                                break
                        }
                }
                cls := "btn btn-secondary btn-sm"
                if active {
                        cls = "btn btn-sm"
                }
                fmt.Fprintf(&b, `<a href="/jobs?status=%s" class="%s">%s</a>`, s, cls, s)
        }
        b.WriteString(`</div>`)

        if len(jobs) == 0 {
                b.WriteString(`<div class="empty">No jobs found.</div>`)
        } else {
                b.WriteString(`<table><thead><tr><th>ID</th><th>Name</th><th>Command</th><th>Target</th><th>Priority</th><th>Status</th><th>Created</th></tr></thead><tbody>`)
                for _, j := range jobs {
                        display := j.Command
                        if display == "" && j.Script.Valid {
                                display = "[script: " + j.ScriptLanguage.String + "]"
                        }
                        target := "any"
                        if j.TargetWorkerID.Valid {
                                target = shortID(j.TargetWorkerID.String)
                        }
                        fmt.Fprintf(&b, `<tr><td class="mono">%s</td><td><a href="/jobs/%s">%s</a></td><td class="mono">%s</td><td class="mono">%s</td><td>%d</td><td><span class="%s">%s</span></td><td>%s</td></tr>`,
                                shortID(j.ID), j.ID, j.Name, display, target, j.Priority, badgeClass(j.Status), j.Status, formatTime(j.CreatedAt))
                }
                b.WriteString(`</tbody></table>`)
        }

        if total > limit {
                b.WriteString(`<div style="margin-top:16px;display:flex;justify-content:space-between;align-items:center">`)
                if offset > 0 {
                        prev := offset - limit
                        if prev < 0 {
                                prev = 0
                        }
                        fmt.Fprintf(&b, `<a href="/jobs?limit=%d&offset=%d" class="btn btn-secondary btn-sm">Previous</a>`, limit, prev)
                } else {
                        b.WriteString(`<span></span>`)
                }
                fmt.Fprintf(&b, `<span class="hint">Showing %d-%d of %d</span>`, offset+1, offset+len(jobs), total)
                if offset+limit < total {
                        fmt.Fprintf(&b, `<a href="/jobs?limit=%d&offset=%d" class="btn btn-secondary btn-sm">Next</a>`, limit, offset+limit)
                }
                b.WriteString(`</div>`)
        }
        b.WriteString(`</div>`)
        return b.String()
}

func newJobContent(workers []*store.Worker) string {
        var b strings.Builder
        b.WriteString(`<div class="page-header"><h2>Submit Job</h2></div>
<div class="card">
<form method="POST" action="/jobs" enctype="multipart/form-data">
  <div class="row">
    <div class="form-group">
      <label for="name">Job Name</label>
      <input id="name" name="name" placeholder="e.g. nightly-backup" required>
    </div>
    <div class="form-group">
      <label for="idempotency_key">Idempotency Key (optional)</label>
      <input id="idempotency_key" name="idempotency_key" placeholder="unique-key-to-deduplicate">
    </div>
  </div>

  <div class="form-group">
    <label>Execution Mode</label>
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-top:4px">
      <label style="display:flex;align-items:center;gap:6px;font-weight:normal;cursor:pointer">
        <input type="radio" name="mode" value="command" checked onchange="toggleMode()">
        <span>Command (existing binary/script)</span>
      </label>
      <label style="display:flex;align-items:center;gap:6px;font-weight:normal;cursor:pointer">
        <input type="radio" name="mode" value="inline" onchange="toggleMode()">
        <span>Inline Script (bash/python)</span>
      </label>
      <label style="display:flex;align-items:center;gap:6px;font-weight:normal;cursor:pointer">
        <input type="radio" name="mode" value="upload" onchange="toggleMode()">
        <span>Upload .sh / .ps1 / .py</span>
      </label>
    </div>
  </div>

  <div id="command-section" class="form-group">
    <label for="command">Command</label>
    <input id="command" name="command" placeholder="/usr/local/bin/backup.sh" class="mono">
    <div class="hint">Absolute path to an executable binary or script</div>
  </div>

  <div id="arguments-row" class="form-group">
    <label for="arguments">Arguments</label>
    <input id="arguments" name="arguments" placeholder="--verbose --compress" class="mono">
  </div>

  <div id="inline-section" class="form-group" style="display:none">
    <label for="script">Inline Script</label>
    <textarea id="script" name="script" placeholder="#!/bin/bash&#10;echo &quot;Hello from Runbooks&quot;&#10;df -h" style="min-height:200px"></textarea>
  </div>

  <div id="language-row" class="form-group" style="display:none">
    <label for="script_language">Script Language</label>
    <select id="script_language" name="script_language">
      <option value="bash">Bash</option>
      <option value="sh">Sh</option>
      <option value="python">Python 3</option>
      <option value="powershell">PowerShell</option>
    </select>
  </div>

  <div id="upload-section" class="form-group" style="display:none">
    <label for="script_file">Upload Script File</label>
    <input id="script_file" name="script_file" type="file" accept=".sh,.bash,.py,.ps1">
    <div class="hint">File will be stored as the script body</div>
  </div>

  <div class="form-group">
    <label for="target_worker_id">Target Worker (optional)</label>
    <select id="target_worker_id" name="target_worker_id">
      <option value="">Any available worker</option>`)
        for _, w := range workers {
                if w.Status == "ACTIVE" {
                        fmt.Fprintf(&b, `<option value="%s">%s</option>`, w.ID, w.Name)
                }
        }
        b.WriteString(`    </select>
    <div class="hint">Restrict this job to a specific worker host</div>
  </div>

  <div class="form-group">
    <label for="environment">Environment Variables (KEY=value per line)</label>
    <textarea id="environment" name="environment" placeholder="AWS_REGION=us-east-1&#10;LOG_LEVEL=info"></textarea>
  </div>
  <div class="row">
    <div class="form-group">
      <label for="priority">Priority (higher runs first)</label>
      <input id="priority" name="priority" type="number" value="0" placeholder="0">
    </div>
    <div class="form-group">
      <label for="timeout_seconds">Timeout (seconds)</label>
      <input id="timeout_seconds" name="timeout_seconds" type="number" value="3600" placeholder="3600">
    </div>
    <div class="form-group">
      <label for="max_retries">Max Retries</label>
      <input id="max_retries" name="max_retries" type="number" value="0" placeholder="0">
    </div>
  </div>
  <div class="form-group">
    <label for="created_by">Created By (optional)</label>
    <input id="created_by" name="created_by" placeholder="operator@example.com">
  </div>
  <button type="submit" class="btn">Submit Job</button>
  <a href="/jobs" class="btn btn-secondary">Cancel</a>
</form>
</div>
<script>
function toggleMode() {
  var mode = document.querySelector('input[name="mode"]:checked').value;
  document.getElementById('command-section').style.display = (mode === 'command') ? '' : 'none';
  document.getElementById('arguments-row').style.display = (mode === 'command') ? '' : 'none';
  document.getElementById('inline-section').style.display = (mode === 'inline') ? '' : 'none';
  document.getElementById('language-row').style.display = (mode === 'inline' || mode === 'upload') ? '' : 'none';
  document.getElementById('upload-section').style.display = (mode === 'upload') ? '' : 'none';
  if (mode === 'upload') {
    document.getElementById('language-row').style.display = '';
  }
}
document.getElementById('script_file').addEventListener('change', function(e) {
  var f = e.target.files[0];
  if (!f) return;
  var ext = f.name.split('.').pop().toLowerCase();
  var lang = document.getElementById('script_language');
  if (ext === 'ps1') lang.value = 'powershell';
  else if (ext === 'py') lang.value = 'python';
  else lang.value = 'bash';
});
</script>`)
        return b.String()
}

func jobDetailContent(j *store.Job, attempts []*store.Attempt, workerNames map[string]string, segments []*store.LogSegment) string {
        var b strings.Builder
        fmt.Fprintf(&b, `<div class="page-header"><h2>%s</h2><div>`, j.Name)
        if j.Status == "FAILED" || j.Status == "LOST" {
                fmt.Fprintf(&b, `<form method="POST" action="/jobs/%s/retry" style="display:inline"><button type="submit" class="btn btn-secondary">Retry</button></form>`, j.ID)
        }
        b.WriteString(`</div></div>`)
        b.WriteString(`<div class="card">`)
        detailRow(&b, "Job ID", j.ID, true)
        if j.Command != "" {
                detailRow(&b, "Command", j.Command, true)
        }
        if j.Arguments != "" {
                detailRow(&b, "Arguments", j.Arguments, true)
        }
        if j.Script.Valid {
                lang := j.ScriptLanguage.String
                if lang == "" {
                        lang = "bash"
                }
                detailRow(&b, "Script Language", lang, false)
        }
        if j.TargetWorkerID.Valid {
                detailRow(&b, "Target Worker", j.TargetWorkerID.String, true)
        }
        if j.Environment != "" {
                detailRow(&b, "Environment", j.Environment, true)
        }
        detailRow(&b, "Status", fmt.Sprintf(`<span class="%s">%s</span>`, badgeClass(j.Status), j.Status), false)
        detailRow(&b, "Priority", fmt.Sprintf("%d", j.Priority), false)
        detailRow(&b, "Timeout", fmt.Sprintf("%ds", j.TimeoutSeconds), false)
        detailRow(&b, "Retries", fmt.Sprintf("%d / %d", j.RetryCount, j.MaxRetries), false)
        if j.IdempotencyKey.Valid {
                detailRow(&b, "Idempotency Key", j.IdempotencyKey.String, true)
        }
        if j.ScheduleID.Valid {
                detailRow(&b, "Schedule ID", j.ScheduleID.String, true)
        }
        if j.CreatedBy.Valid {
                detailRow(&b, "Created By", j.CreatedBy.String, false)
        }
        detailRow(&b, "Created At", formatTime(j.CreatedAt), false)
        if j.StartedAt.Valid {
                detailRow(&b, "Started At", formatTime(j.StartedAt.Time), false)
        }
        if j.CompletedAt.Valid {
                detailRow(&b, "Completed At", formatTime(j.CompletedAt.Time), false)
        }
        b.WriteString(`</div>`)

        if j.Script.Valid {
                b.WriteString(`<div class="card"><h3 style="margin:0 0 12px 0;font-size:14px;color:#334155">Script</h3>`)
                b.WriteString(`<pre class="log-viewer" style="max-height:300px">`)
                b.WriteString(template.HTMLEscapeString(j.Script.String))
                b.WriteString(`</pre></div>`)
        }

        b.WriteString(`<div class="card"><h3 style="margin:0 0 12px 0;font-size:14px;color:#334155">Attempt History</h3>`)
        if len(attempts) == 0 {
                b.WriteString(`<div class="empty">No attempts yet.</div>`)
        } else {
                b.WriteString(`<table><thead><tr><th>#</th><th>Worker</th><th>Status</th><th>Started</th><th>Completed</th><th>Exit Code</th><th>Failure Reason</th></tr></thead><tbody>`)
                for _, a := range attempts {
                        workerDisplay := "-"
                        if a.WorkerID.Valid {
                                if name, ok := workerNames[a.WorkerID.String]; ok {
                                        workerDisplay = fmt.Sprintf(`<span title="%s">%s</span>`, a.WorkerID.String, name)
                                } else {
                                        workerDisplay = shortID(a.WorkerID.String)
                                }
                        }
                        exitCode := "-"
                        if a.ExitCode.Valid {
                                exitCode = fmt.Sprintf("%d", a.ExitCode.Int64)
                        }
                        reason := "-"
                        if a.FailureReason.Valid {
                                reason = a.FailureReason.String
                        }
                        fmt.Fprintf(&b, `<tr><td>%d</td><td>%s</td><td><span class="%s">%s</span></td><td>%s</td><td>%s</td><td>%s</td><td class="mono">%s</td></tr>`,
                                a.AttemptNumber, workerDisplay, badgeClass(a.Status), a.Status, formatTime(a.StartedAt), formatTime(a.CompletedAt.Time), exitCode, reason)
                }
                b.WriteString(`</tbody></table>`)
        }
        b.WriteString(`</div>`)

        if len(segments) > 0 {
                b.WriteString(`<div class="card"><h3 style="margin:0 0 12px 0;font-size:14px;color:#334155">Logs</h3>`)
                b.WriteString(`<div class="log-viewer" id="log-viewer" data-job-id="` + j.ID + `">Loading...</div>`)
                b.WriteString(`</div>`)
        }
        return b.String()
}

func detailRow(b *strings.Builder, label, value string, mono bool) {
        b.WriteString(`<div class="detail-row"><div class="label">`)
        b.WriteString(label)
        b.WriteString(`</div><div class="value">`)
        if mono {
                b.WriteString(`<span class="mono">`)
                b.WriteString(value)
                b.WriteString(`</span>`)
        } else {
                b.WriteString(value)
        }
        b.WriteString(`</div></div>`)
}

func workersListContent(workers []*store.Worker) string {
        var b strings.Builder
        b.WriteString(`<div class="page-header"><h2>Workers</h2></div>`)
        b.WriteString(`<div class="card">`)
        if len(workers) == 0 {
                b.WriteString(`<div class="empty">No workers registered.</div>`)
        } else {
                b.WriteString(`<table><thead><tr><th>Name</th><th>Status</th><th>Concurrency</th><th>Last Heartbeat</th><th>Registered</th></tr></thead><tbody>`)
                for _, w := range workers {
                        fmt.Fprintf(&b, `<tr><td>%s</td><td><span class="%s">%s</span></td><td>%d</td><td>%s</td><td>%s</td></tr>`,
                                w.Name, badgeClass(w.Status), w.Status, w.MaxConcurrency, formatTime(w.LastHeartbeatAt.Time), formatTime(w.RegisteredAt))
                }
                b.WriteString(`</tbody></table>`)
        }
        b.WriteString(`</div>`)
        return b.String()
}

func schedulesListContent(schedules []*store.Schedule) string {
        var b strings.Builder
        b.WriteString(`<div class="page-header"><h2>Schedules</h2><a href="/schedules/new" class="btn">New Schedule</a></div>`)
        b.WriteString(`<div class="card">`)
        if len(schedules) == 0 {
                b.WriteString(`<div class="empty">No schedules configured.</div>`)
        } else {
                b.WriteString(`<table><thead><tr><th>Name</th><th>Cron</th><th>Enabled</th><th>Overlap Protection</th><th>Last Run</th><th>Next Run</th></tr></thead><tbody>`)
                for _, s := range schedules {
                        enabled := "Yes"
                        if !s.Enabled {
                                enabled = "No"
                        }
                        overlap := "Yes"
                        if !s.PreventOverlap {
                                overlap = "No"
                        }
                        fmt.Fprintf(&b, `<tr><td>%s</td><td class="mono">%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
                                s.Name, s.CronExpression, enabled, overlap, formatTime(s.LastRunAt.Time), formatTime(s.NextRunAt.Time))
                }
                b.WriteString(`</tbody></table>`)
        }
        b.WriteString(`</div>`)
        return b.String()
}

func newScheduleContent() string {
        return `<div class="page-header"><h2>New Schedule</h2></div>
<div class="card">
<form method="POST" action="/schedules">
  <div class="form-group">
    <label for="schedule_name">Schedule Name</label>
    <input id="schedule_name" name="schedule_name" placeholder="e.g. nightly-backup-schedule" required>
  </div>
  <div class="form-group">
    <label for="cron_expression">Cron Expression</label>
    <input id="cron_expression" name="cron_expression" placeholder="* * * * *  (min hour day month weekday)" class="mono" required>
    <div class="hint">5-field POSIX cron syntax</div>
  </div>
  <div class="row">
    <div class="form-group">
      <label for="command">Command</label>
      <input id="command" name="command" placeholder="/usr/local/bin/backup.sh" class="mono" required>
    </div>
    <div class="form-group">
      <label for="arguments">Arguments</label>
      <input id="arguments" name="arguments" placeholder="--verbose" class="mono">
    </div>
  </div>
  <div class="row">
    <div class="form-group">
      <label for="timeout_seconds">Timeout (seconds)</label>
      <input id="timeout_seconds" name="timeout_seconds" type="number" value="3600">
    </div>
    <div class="form-group">
      <label for="max_retries">Max Retries</label>
      <input id="max_retries" name="max_retries" type="number" value="0">
    </div>
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="enabled" checked> Enabled</label>
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="prevent_overlap" checked> Prevent Overlap (skip if previous run still active)</label>
  </div>
  <button type="submit" class="btn">Create Schedule</button>
  <a href="/schedules" class="btn btn-secondary">Cancel</a>
</form>
</div>`
}
