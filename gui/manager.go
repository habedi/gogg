package gui

import (
	"context"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type DownloadTask struct {
	ID         int
	Title      string
	Status     binding.String
	Progress   binding.Float
	CancelFunc context.CancelFunc
	// This will hold the text for concurrent, per-file progress.
	FileStatus binding.String
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
			title := widget.NewLabel("Game Title")
			title.TextStyle = fyne.TextStyle{Bold: true}
			cancelBtn := widget.NewButton("Cancel", nil)
			status := widget.NewLabel("Status")
			status.Wrapping = fyne.TextWrapWord
			progress := widget.NewProgressBar()
			fileStatus := widget.NewLabel("")
			fileStatus.TextStyle = fyne.TextStyle{Monospace: true}
			fileStatus.Wrapping = fyne.TextWrapWord

			topRow := container.NewHBox(title, layout.NewSpacer(), cancelBtn)
			content := container.NewVBox(topRow, status, progress, fileStatus)

			// Wrap the content in a Card to help with layout management.
			return widget.NewCard("", "", content)
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			taskRaw, err := item.(binding.Untyped).Get()
			if err != nil {
				return
			}
			task := taskRaw.(*DownloadTask)

			// The object is now a Card.
			card := obj.(*widget.Card)
			contentVBox := card.Content.(*fyne.Container)
			topRowHBox := contentVBox.Objects[0].(*fyne.Container)

			title := topRowHBox.Objects[0].(*widget.Label)
			cancelBtn := topRowHBox.Objects[2].(*widget.Button)
			status := contentVBox.Objects[1].(*widget.Label)
			progress := contentVBox.Objects[2].(*widget.ProgressBar)
			fileStatus := contentVBox.Objects[3].(*widget.Label)

			title.SetText(task.Title)
			status.Bind(task.Status)
			progress.Bind(task.Progress)
			fileStatus.Bind(task.FileStatus)

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
			if !strings.HasPrefix(status, "Completed") && status != "Cancelled" && !strings.HasPrefix(status, "Error") {
				keptTasks = append(keptTasks, task)
			}
		}
		_ = dm.Tasks.Set(keptTasks)
	})

	return container.NewBorder(nil, clearBtn, nil, nil, list)
}
