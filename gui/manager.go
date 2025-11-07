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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
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
	InstanceID   time.Time // Unique identifier for this specific download
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
	InstanceID   time.Time `json:"instance_id"`
	State        int       `json:"state"`
	Title        string    `json:"title"`
	StatusText   string    `json:"status_text"`
	DownloadPath string    `json:"download_path"`
}

type DownloadManager struct {
	mu          sync.RWMutex
	Tasks       binding.UntypedList
	historyPath fyne.URI
	queue       []queuedDownload
}

type queuedDownload struct {
	authService     *auth.Service
	game            db.Game
	downloadPath    string
	language        string
	platformName    string
	extrasFlag      bool
	dlcFlag         bool
	resumeFlag      bool
	flattenFlag     bool
	skipPatchesFlag bool
	numThreads      int
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
	defer func() { _ = reader.Close() }()

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
			InstanceID:   pTask.InstanceID,
			State:        pTask.State,
			Title:        pTask.Title,
			Status:       status,
			Progress:     progress,
			DownloadPath: pTask.DownloadPath,
			Details:      binding.NewString(),
			FileStatus:   binding.NewString(),
			CancelFunc:   nil,
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
				InstanceID:   task.InstanceID,
				State:        task.State,
				Title:        task.Title,
				StatusText:   status,
				DownloadPath: task.DownloadPath,
			})
		}
	}

	writer, err := storage.Writer(dm.historyPath)
	if err != nil {
		log.Error().Err(err).Msg("Failed to open history file for writing.")
		return
	}
	defer func() { _ = writer.Close() }()

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
			title := widget.NewLabel("Game Title")
			title.TextStyle = fyne.TextStyle{Bold: true}
			title.Truncation = fyne.TextTruncateEllipsis

			actionBtn := widget.NewButtonWithIcon("Action", theme.CancelIcon(), nil)
			clearBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)
			clearBtn.Importance = widget.LowImportance

			actionBox := container.NewHBox(actionBtn, clearBtn)
			topRow := container.NewBorder(nil, nil, nil, actionBox, title)

			status := widget.NewLabel("Status")
			status.Wrapping = fyne.TextWrapWord
			details := widget.NewLabel("Details")
			details.TextStyle = fyne.TextStyle{Italic: true}
			details.Wrapping = fyne.TextWrapWord
			progress := widget.NewProgressBar()

			fileStatus := widget.NewLabel("")
			fileStatus.TextStyle = fyne.TextStyle{Monospace: true}
			fileStatus.Wrapping = fyne.TextWrapOff
			fileStatus.Truncation = fyne.TextTruncateClip

			// Wrap fileStatus in a scroll container with fixed max height
			fileStatusScroll := container.NewVScroll(fileStatus)
			fileStatusScroll.SetMinSize(fyne.NewSize(0, 60))

			progressBox := container.NewVBox(details, progress)
			separator := widget.NewSeparator()

			// Add padding between sections
			paddedFileStatus := container.NewPadded(fileStatusScroll)

			content := container.NewVBox(
				topRow,
				widget.NewSeparator(),
				status,
				progressBox,
				paddedFileStatus,
			)

			// Add padding inside the card
			paddedContent := container.NewPadded(content)
			card := widget.NewCard("", "", paddedContent)

			// Add extra padding and separator between cards
			cardWithSeparator := container.NewVBox(
				card,
				separator,
				layout.NewSpacer(),
			)

			return cardWithSeparator
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			taskRaw, err := item.(binding.Untyped).Get()
			if err != nil {
				return
			}
			task := taskRaw.(*DownloadTask)

			// Navigate: cardWithSeparator -> card -> paddedContent -> content
			cardWithSeparator := obj.(*fyne.Container)
			card := cardWithSeparator.Objects[0].(*widget.Card)
			paddedContent := card.Content.(*fyne.Container)
			contentVBox := paddedContent.Objects[0].(*fyne.Container)

			// Extract elements from new structure
			topRow := contentVBox.Objects[0].(*fyne.Container)
			// Objects[1] is separator
			status := contentVBox.Objects[2].(*widget.Label)
			progressBox := contentVBox.Objects[3].(*fyne.Container)
			paddedFileStatus := contentVBox.Objects[4].(*fyne.Container)

			actionBox := topRow.Objects[1].(*fyne.Container)
			title := topRow.Objects[0].(*widget.Label)
			actionBtn := actionBox.Objects[0].(*widget.Button)
			clearBtn := actionBox.Objects[1].(*widget.Button)

			details := progressBox.Objects[0].(*widget.Label)
			progress := progressBox.Objects[1].(*widget.ProgressBar)

			// Navigate to fileStatus: paddedFileStatus -> scroll -> label
			fileStatusScroll := paddedFileStatus.Objects[0].(*container.Scroll)
			fileStatus := fileStatusScroll.Content.(*widget.Label)

			title.SetText(task.Title)
			status.Bind(task.Status)
			details.Bind(task.Details)
			progress.Bind(task.Progress)
			fileStatus.Bind(task.FileStatus)

			clearBtn.OnTapped = func() {
				dm.mu.Lock()
				currentTasks, _ := dm.Tasks.Get()
				keptTasks := make([]interface{}, 0)
				for _, tRaw := range currentTasks {
					if tRaw.(*DownloadTask).InstanceID != task.InstanceID {
						keptTasks = append(keptTasks, tRaw)
					}
				}
				_ = dm.Tasks.Set(keptTasks)
				dm.mu.Unlock()
				dm.PersistHistory()
			}

			switch task.State {
			case StateCompleted:
				actionBtn.SetIcon(theme.FolderOpenIcon())
				actionBtn.SetText("Open Folder")
				actionBtn.OnTapped = func() { openFolder(task.DownloadPath) }
				actionBtn.Enable()
				clearBtn.Show()
			case StateCancelled, StateError:
				actionBtn.SetIcon(theme.ErrorIcon())
				actionBtn.SetText("Error")
				if task.State == StateCancelled {
					actionBtn.SetIcon(theme.CancelIcon())
					actionBtn.SetText("Cancelled")
				}
				actionBtn.OnTapped = nil
				actionBtn.Disable()
				clearBtn.Show()
			default: // Preparing, Downloading
				actionBtn.SetIcon(theme.CancelIcon())
				actionBtn.SetText("Cancel")
				actionBtn.OnTapped = func() {
					if task.CancelFunc != nil {
						task.CancelFunc()
					}
				}
				actionBtn.Enable()
				clearBtn.Hide()
			}
		},
	)

	clearAllBtn := widget.NewButton("Clear All Finished", func() {
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
		dm.mu.Unlock()
		dm.PersistHistory()
	})
	bottomBar := container.NewHBox(layout.NewSpacer(), clearAllBtn)

	return container.NewBorder(nil, bottomBar, nil, nil, list)
}

func (dm *DownloadManager) activeCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	all, _ := dm.Tasks.Get()
	c := 0
	for _, tRaw := range all {
		t := tRaw.(*DownloadTask)
		switch t.State {
		case StateDownloading:
			c++
		case StatePreparing:
			if status, err := t.Status.Get(); err == nil && status == "Queued" {
				continue
			}
			c++
		case StateCompleted, StateCancelled, StateError:
			// not active
		}
	}
	return c
}

func (dm *DownloadManager) maxConcurrent() int {
	return fyne.CurrentApp().Preferences().IntWithFallback("download.maxConcurrent", 2)
}

func (dm *DownloadManager) QueueOrStart(q queuedDownload) error {
	// Prevent duplicate active or queued
	dm.mu.RLock()
	all, _ := dm.Tasks.Get()
	for _, tRaw := range all {
		t := tRaw.(*DownloadTask)
		if t.ID == q.game.ID && (t.State == StatePreparing || t.State == StateDownloading) {
			dm.mu.RUnlock()
			return ErrDownloadInProgress
		}
	}
	dm.mu.RUnlock()
	if dm.activeCount() < dm.maxConcurrent() {
		return executeDownload(q.authService, dm, q.game, q.downloadPath, q.language, q.platformName, q.extrasFlag, q.dlcFlag, q.resumeFlag, q.flattenFlag, q.skipPatchesFlag, q.numThreads)
	}
	// Enqueue
	dm.mu.Lock()
	dm.queue = append(dm.queue, q)
	// Add placeholder task
	placeholder := &DownloadTask{
		ID:         q.game.ID,
		InstanceID: time.Now(),
		Title:      q.game.Title,
		State:      StatePreparing,
		Status:     binding.NewString(),
		Details:    binding.NewString(),
		Progress:   binding.NewFloat(),
		FileStatus: binding.NewString(),
	}
	_ = placeholder.Status.Set("Queued")
	_ = dm.Tasks.Append(placeholder)
	dm.mu.Unlock()
	return nil
}

func (dm *DownloadManager) startNextIfAvailable() {
	for {
		if dm.activeCount() >= dm.maxConcurrent() {
			return
		}
		dm.mu.Lock()
		if len(dm.queue) == 0 {
			dm.mu.Unlock()
			return
		}
		next := dm.queue[0]
		dm.queue = dm.queue[1:]
		// Remove any queued placeholder for this game
		all, _ := dm.Tasks.Get()
		filtered := make([]interface{}, 0, len(all))
		for _, tRaw := range all {
			t := tRaw.(*DownloadTask)
			if t.ID == next.game.ID && t.State == StatePreparing {
				status, _ := t.Status.Get()
				if status == "Queued" {
					continue
				}
			}
			filtered = append(filtered, tRaw)
		}
		_ = dm.Tasks.Set(filtered)
		dm.mu.Unlock()
		_ = executeDownload(next.authService, dm, next.game, next.downloadPath, next.language, next.platformName, next.extrasFlag, next.dlcFlag, next.resumeFlag, next.flattenFlag, next.skipPatchesFlag, next.numThreads)
	}
}
