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

func (dm *DownloadManager) AddTask(task *DownloadTask) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.Tasks.Append(task)
}

func DownloadsTabUI(dm *DownloadManager) fyne.CanvasObject {
	list := widget.NewListWithData(
		dm.Tasks,
		func() fyne.CanvasObject {
			progress := widget.NewProgressBar()
			status := widget.NewLabel("Status")
			status.Wrapping = fyne.TextWrapWord
			cancelBtn := widget.NewButton("Cancel", nil)
			return container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Game Title"),
				cancelBtn,
				container.NewVBox(status, progress),
			)
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			taskRaw, _ := item.(binding.Untyped).Get()
			task := taskRaw.(*DownloadTask)

			title := obj.(*fyne.Container).Objects[1].(*widget.Label)
			cancelBtn := obj.(*fyne.Container).Objects[2].(*widget.Button)
			vbox := obj.(*fyne.Container).Objects[0].(*fyne.Container)
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
			isFinalState := (s == "Completed" || s == "Cancelled" || strings.HasPrefix(s, "Error"))
			if isFinalState {
				cancelBtn.Disable()
			} else {
				cancelBtn.Enable()
			}
		},
	)
	return list
}
