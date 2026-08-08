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

// downloadsHeadline is the line above the list for any state, not only while
// something is running: an active summary when there is one, and otherwise a
// count of what has finished, so a list of completed downloads is not sat
// under a blank bar.
func downloadsHeadline(tasks []*DownloadTask) string {
	active, downloaded, total, speed := downloadTotals(tasks)
	if active > 0 {
		return totalsSummary(active, downloaded, total, speed)
	}

	var done, failed, paused int
	for _, task := range tasks {
		switch task.State() {
		case StateCompleted:
			done++
		case StateCancelled, StateError:
			failed++
		case StatePaused:
			paused++
		}
	}

	var parts []string
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d finished", done))
	}
	if paused > 0 {
		parts = append(parts, fmt.Sprintf("%d paused", paused))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	return strings.Join(parts, " · ")
}

// totals is the aggregate line the Downloads tab shows. It is created on first
// use so a manager built without a constructor still works.
func (dm *DownloadManager) totals() binding.String {
	dm.totalsOnce.Do(func() { dm.totalsBinding = binding.NewString() })
	return dm.totalsBinding
}

// refreshTotals recomputes the aggregate line from the current downloads.
func (dm *DownloadManager) refreshTotals() {
	_ = dm.totals().Set(downloadsHeadline(dm.tasksSnapshot()))
}
