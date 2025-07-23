package gui

import (
	"context"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

type DownloadTask struct {
	ID         int
	Title      string
	Status     binding.String
	Progress   binding.Float
	CancelFunc context.CancelFunc
}

type DownloadManager struct {
	mu    sync.RWMutex
	Tasks binding.UntypedList
}

func NewDownloadManager() *DownloadManager {
	return &DownloadManager{
		Tasks: binding.NewUntypedList(),
	}
}

func (dm *DownloadManager) AddTask(task *DownloadTask) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.Tasks.Append(task)
}

func DownloadsTabUI(dm *DownloadManager) fyne.CanvasObject {
	list := widget.NewListWithData(
		dm.Tasks,
		func() fyne.CanvasObject {
			progress := widget.NewProgressBar()
			status := widget.NewLabel("Status")
			status.Wrapping = fyne.TextWrapWord
			cancelBtn := widget.NewButton("Cancel", nil)
			title := widget.NewLabel("Game Title")
			title.TextStyle = fyne.TextStyle{Bold: true}

			return container.NewBorder(
				title,                               // Top
				nil,                                 // Bottom
				nil,                                 // Left
				cancelBtn,                           // Right
				container.NewVBox(status, progress), // Center
			)
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			taskRaw, _ := item.(binding.Untyped).Get()
			task := taskRaw.(*DownloadTask)

			border := obj.(*fyne.Container)

			// Correctly access widgets. Fyne's Border layout only adds non-nil
			// objects to its list. Given the layout above, the list is:
			// [0] = center, [1] = top, [2] = right
			vbox := border.Objects[0].(*fyne.Container)
			title := border.Objects[1].(*widget.Label)
			cancelBtn := border.Objects[2].(*widget.Button)

			status := vbox.Objects[0].(*widget.Label)
			progress := vbox.Objects[1].(*widget.ProgressBar)

			title.SetText(task.Title)
			status.Bind(task.Status)
			progress.Bind(task.Progress)

			cancelBtn.OnTapped = func() {
				if task.CancelFunc != nil {
					task.CancelFunc()
				}
				cancelBtn.Disable()
			}

			s, _ := task.Status.Get()
			isFinalState := strings.HasPrefix(s, "Completed") || s == "Cancelled" || strings.HasPrefix(s, "Error")
			if isFinalState {
				cancelBtn.Disable()
			} else {
				cancelBtn.Enable()
			}
		},
	)

	clearBtn := widget.NewButton("Clear Finished", func() {
		dm.mu.Lock()
		defer dm.mu.Unlock()

		currentTasks, _ := dm.Tasks.Get()
		keptTasks := make([]interface{}, 0)

		for _, taskRaw := range currentTasks {
			task := taskRaw.(*DownloadTask)
			status, _ := task.Status.Get()
			if !(strings.HasPrefix(status, "Completed") || status == "Cancelled" || strings.HasPrefix(status, "Error")) {
				keptTasks = append(keptTasks, task)
			}
		}
		dm.Tasks.Set(keptTasks)
	})

	return container.NewBorder(nil, clearBtn, nil, nil, list)
}
