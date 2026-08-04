package gui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2/data/binding"
)

// downloadTotals sums the progress of downloads that are still running.
func downloadTotals(tasks []*DownloadTask) (active int, downloaded, total, speed int64) {
	for _, task := range tasks {
		switch task.State() {
		case StatePreparing, StateDownloading:
		default:
			continue
		}
		active++
		taskDownloaded, taskTotal, taskSpeed := task.ProgressBytes()
		downloaded += taskDownloaded
		total += taskTotal
		speed += taskSpeed
	}
	return active, downloaded, total, speed
}

// totalsSummary renders the aggregate line shown above the download list. It is
// empty when nothing is running, so the header takes no room.
func totalsSummary(active int, downloaded, total, speed int64) string {
	if active == 0 {
		return ""
	}

	parts := []string{fmt.Sprintf("%d %s", active, downloadsWord(active))}
	if total > 0 {
		parts = append(parts, fmt.Sprintf("%s of %s (%d%%)",
			formatBytes(downloaded), formatBytes(total), downloaded*100/total))
	} else {
		parts = append(parts, formatBytes(downloaded))
	}

	if speed > 0 {
		parts = append(parts, fmt.Sprintf("%s/s", formatBytes(speed)))
		if remaining := total - downloaded; remaining > 0 {
			eta := time.Duration(float64(remaining)/float64(speed)) * time.Second
			parts = append(parts, "ETA "+eta.Truncate(time.Second).String())
		}
	}

	return strings.Join(parts, " · ")
}

func downloadsWord(n int) string {
	if n == 1 {
		return "download"
	}
	return "downloads"
}

// totals is the aggregate line the Downloads tab shows. It is created on first
// use so a manager built without a constructor still works.
func (dm *DownloadManager) totals() binding.String {
	dm.totalsOnce.Do(func() { dm.totalsBinding = binding.NewString() })
	return dm.totalsBinding
}

// refreshTotals recomputes the aggregate line from the current downloads.
func (dm *DownloadManager) refreshTotals() {
	dm.mu.RLock()
	all, _ := dm.Tasks.Get()
	tasks := make([]*DownloadTask, 0, len(all))
	for _, raw := range all {
		if task, ok := raw.(*DownloadTask); ok {
			tasks = append(tasks, task)
		}
	}
	dm.mu.RUnlock()

	_ = dm.totals().Set(totalsSummary(downloadTotals(tasks)))
}
