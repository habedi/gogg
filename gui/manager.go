package gui

import (
	"context"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	StatePreparing = iota
	StateDownloading
	StateCompleted
	StateCancelled
	StateError
)

type DownloadTask struct {
	ID           int
	State        int
	Title        string
	Status       binding.String
	Details      binding.String
	Progress     binding.Float
	CancelFunc   context.CancelFunc
	FileStatus   binding.String
	DownloadPath string // Path for the "Open Folder" button
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
			actionBtn := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), nil)
			status := widget.NewLabel("Status")
			status.Wrapping = fyne.TextWrapWord
			details := widget.NewLabel("Details")
			details.TextStyle = fyne.TextStyle{Italic: true}
			progress := widget.NewProgressBar()
			fileStatus := widget.NewLabel("")
			fileStatus.TextStyle = fyne.TextStyle{Monospace: true}
			fileStatus.Wrapping = fyne.TextWrapWord

			topRow := container.NewBorder(nil, nil, nil, actionBtn, title)
			progressBox := container.NewVBox(details, progress)
			content := container.NewVBox(topRow, status, progressBox, fileStatus)

			return widget.NewCard("", "", content)
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			taskRaw, err := item.(binding.Untyped).Get()
			if err != nil {
				return
			}
			task := taskRaw.(*DownloadTask)

			card := obj.(*widget.Card)
			contentVBox := card.Content.(*fyne.Container)
			topRow := contentVBox.Objects[0].(*fyne.Container)
			progressBox := contentVBox.Objects[2].(*fyne.Container)

			title := topRow.Objects[0].(*widget.Label)
			actionBtn := topRow.Objects[1].(*widget.Button)
			status := contentVBox.Objects[1].(*widget.Label)
			details := progressBox.Objects[0].(*widget.Label)
			progress := progressBox.Objects[1].(*widget.ProgressBar)
			fileStatus := contentVBox.Objects[3].(*widget.Label)

			title.SetText(task.Title)
			status.Bind(task.Status)
			details.Bind(task.Details)
			progress.Bind(task.Progress)
			fileStatus.Bind(task.FileStatus)

			switch task.State {
			case StateCompleted:
				actionBtn.SetIcon(theme.FolderOpenIcon())
				actionBtn.SetText("Open Folder")
				actionBtn.OnTapped = func() { openFolder(task.DownloadPath) }
				actionBtn.Enable()
			case StateCancelled:
				actionBtn.SetIcon(theme.CancelIcon())
				actionBtn.SetText("Cancelled")
				actionBtn.OnTapped = nil
				actionBtn.Disable()
			case StateError:
				actionBtn.SetIcon(theme.ErrorIcon())
				actionBtn.SetText("Error")
				actionBtn.OnTapped = nil
				actionBtn.Disable()
			default: // Preparing, Downloading
				actionBtn.SetIcon(theme.CancelIcon())
				actionBtn.SetText("Cancel")
				actionBtn.OnTapped = func() {
					if task.CancelFunc != nil {
						task.CancelFunc()
					}
				}
				actionBtn.Enable()
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
			if task.State != StateCompleted && task.State != StateCancelled && task.State != StateError {
				keptTasks = append(keptTasks, task)
			}
		}
		_ = dm.Tasks.Set(keptTasks)
	})

	return container.NewBorder(nil, clearBtn, nil, nil, list)
}
