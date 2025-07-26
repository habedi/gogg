package gui

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/rs/zerolog/log"
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
	DownloadPath string
}

// PersistentDownloadTask is a serializable representation of a finished task.
type PersistentDownloadTask struct {
	ID           int       `json:"id"`
	State        int       `json:"state"`
	Title        string    `json:"title"`
	StatusText   string    `json:"status_text"`
	DownloadPath string    `json:"download_path"`
	Timestamp    time.Time `json:"timestamp"`
}

type DownloadManager struct {
	mu          sync.RWMutex
	Tasks       binding.UntypedList
	historyPath fyne.URI
}

func NewDownloadManager() *DownloadManager {
	a := fyne.CurrentApp()
	historyURI, err := storage.Child(a.Storage().RootURI(), "download_history.json")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create history file path")
	}

	dm := &DownloadManager{
		Tasks:       binding.NewUntypedList(),
		historyPath: historyURI,
	}

	dm.loadHistory()
	return dm
}

func (dm *DownloadManager) AddTask(task *DownloadTask) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.Tasks.Append(task)
}

func (dm *DownloadManager) loadHistory() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	reader, err := storage.Reader(dm.historyPath)
	if err != nil {
		log.Info().Msg("No download history found or file is not readable.")
		return
	}
	defer reader.Close()

	bytes, err := io.ReadAll(reader)
	if err != nil || len(bytes) == 0 {
		log.Error().Err(err).Msg("Failed to read history file or file is empty.")
		return
	}

	var persistentTasks []PersistentDownloadTask
	if err := json.Unmarshal(bytes, &persistentTasks); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal download history.")
		return
	}

	uiTasks := make([]interface{}, 0, len(persistentTasks))
	for _, pTask := range persistentTasks {
		status := binding.NewString()
		_ = status.Set(pTask.StatusText)
		progress := binding.NewFloat()
		if pTask.State == StateCompleted {
			_ = progress.Set(1.0)
		}

		uiTasks = append(uiTasks, &DownloadTask{
			ID:           pTask.ID,
			State:        pTask.State,
			Title:        pTask.Title,
			Status:       status,
			Progress:     progress,
			DownloadPath: pTask.DownloadPath,
			// Non-persistent fields are left as zero-values
			Details:    binding.NewString(),
			FileStatus: binding.NewString(),
			CancelFunc: nil,
		})
	}
	_ = dm.Tasks.Set(uiTasks)
	log.Info().Int("count", len(uiTasks)).Msg("Download history loaded.")
}

func (dm *DownloadManager) PersistHistory() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	allTasks, _ := dm.Tasks.Get()
	persistentTasks := make([]PersistentDownloadTask, 0)

	for _, taskRaw := range allTasks {
		task := taskRaw.(*DownloadTask)
		if task.State == StateCompleted || task.State == StateCancelled || task.State == StateError {
			status, _ := task.Status.Get()
			persistentTasks = append(persistentTasks, PersistentDownloadTask{
				ID:           task.ID,
				State:        task.State,
				Title:        task.Title,
				StatusText:   status,
				DownloadPath: task.DownloadPath,
				Timestamp:    time.Now(),
			})
		}
	}

	writer, err := storage.Writer(dm.historyPath)
	if err != nil {
		log.Error().Err(err).Msg("Failed to open history file for writing.")
		return
	}
	defer writer.Close()

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(persistentTasks); err != nil {
		log.Error().Err(err).Msg("Failed to encode and save download history.")
	}
}

func DownloadsTabUI(dm *DownloadManager) fyne.CanvasObject {
	list := widget.NewListWithData(
		dm.Tasks,
		func() fyne.CanvasObject {
			// This function remains the same
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
			// This function also remains the same
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

		currentTasks, _ := dm.Tasks.Get()
		keptTasks := make([]interface{}, 0)

		for _, taskRaw := range currentTasks {
			task := taskRaw.(*DownloadTask)
			if task.State != StateCompleted && task.State != StateCancelled && task.State != StateError {
				keptTasks = append(keptTasks, task)
			}
		}
		_ = dm.Tasks.Set(keptTasks)
		dm.mu.Unlock() // Unlock before persisting
		dm.PersistHistory()
	})

	return container.NewBorder(nil, clearBtn, nil, nil, list)
}
