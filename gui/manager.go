package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
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
	// StatePaused is stopped on purpose with the intent to come back: the
	// partial files stay, and Resume picks up where they end. Added after the
	// others because the values are written into the history file.
	StatePaused
)

type DownloadTask struct {
	ID           int
	InstanceID   time.Time // Unique identifier for this specific download
	Title        string
	Status       binding.String
	Details      binding.String
	Progress     binding.Float
	CancelFunc   context.CancelFunc
	FileStatus   binding.String
	DownloadPath string

	// request is what this download was started from, kept so it can be retried.
	// Tasks restored from the history file do not have one.
	request queuedDownload

	// state is written by the download goroutine and read by the UI, so it is
	// only reachable through State and SetState.
	state atomic.Int32

	// pausing marks a cancellation as a pause, so the download goroutine ends
	// the task as paused rather than cancelled. Set before CancelFunc runs.
	pausing atomic.Bool

	// onStateChange is set by the manager when it takes a download on. Nothing
	// else says a download has moved between running and finished, which is
	// what the Downloads tab is ordered by.
	onStateChange func()

	// Byte counters for the aggregate header, written by the download
	// goroutine and read by the UI.
	downloadedBytes atomic.Int64
	totalBytes      atomic.Int64
	speedBytes      atomic.Int64
}

// SetProgressBytes records how far along this download is. Safe for concurrent use.
func (t *DownloadTask) SetProgressBytes(downloaded, total, speed int64) {
	t.downloadedBytes.Store(downloaded)
	t.totalBytes.Store(total)
	t.speedBytes.Store(speed)
}

// ProgressBytes reports how far along this download is. Safe for concurrent use.
func (t *DownloadTask) ProgressBytes() (downloaded, total, speed int64) {
	return t.downloadedBytes.Load(), t.totalBytes.Load(), t.speedBytes.Load()
}

// State returns the current state of the task. Safe for concurrent use.
func (t *DownloadTask) State() int { return int(t.state.Load()) }

// SetState updates the state of the task. Safe for concurrent use.
func (t *DownloadTask) SetState(state int) {
	t.state.Store(int32(state))
	if t.onStateChange != nil {
		t.onStateChange()
	}
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
	mu            sync.RWMutex
	Tasks         binding.UntypedList
	historyPath   fyne.URI
	queue         []queuedDownload
	totalsOnce    sync.Once
	totalsBinding binding.String
	statesOnce    sync.Once
	statesBinding binding.Int

	// How the downloads since the last quiet moment ended, counted so the
	// batch can be announced as one piece of news when the last one lands.
	finishedOK, finishedFailed int

	// active is the games whose download goroutine is running, so a game
	// cannot be downloaded twice at once. Guarded by activeMu, not mu: slots
	// are taken and given back on the download goroutines.
	activeMu sync.Mutex
	active   map[int]struct{}
}

// acquireSlot claims the one download a game may have running. It reports
// false when the game already holds it.
func (dm *DownloadManager) acquireSlot(gameID int) bool {
	dm.activeMu.Lock()
	defer dm.activeMu.Unlock()
	if dm.active == nil {
		dm.active = make(map[int]struct{})
	}
	if _, held := dm.active[gameID]; held {
		return false
	}
	dm.active[gameID] = struct{}{}
	return true
}

// releaseSlot gives a finished download's slot back.
func (dm *DownloadManager) releaseSlot(gameID int) {
	dm.activeMu.Lock()
	defer dm.activeMu.Unlock()
	delete(dm.active, gameID)
}

// slotHeld reports whether a game's download goroutine is still running.
func (dm *DownloadManager) slotHeld(gameID int) bool {
	dm.activeMu.Lock()
	defer dm.activeMu.Unlock()
	_, held := dm.active[gameID]
	return held
}

// noteFinished records how one download ended and answers with what to
// announce: nothing while others are still on their way, and the whole batch
// once the last one lands. Cancelled downloads are not news the user needs
// breaking to them, but they still close a batch out.
func (dm *DownloadManager) noteFinished(state int) (done, failed int, last bool) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	switch state {
	case StateCompleted:
		dm.finishedOK++
	case StateError:
		dm.finishedFailed++
	}

	all, _ := dm.Tasks.Get()
	for _, raw := range all {
		if task, ok := raw.(*DownloadTask); ok && stillGoing(task) {
			return 0, 0, false
		}
	}

	done, failed = dm.finishedOK, dm.finishedFailed
	dm.finishedOK, dm.finishedFailed = 0, 0
	return done, failed, done+failed > 0
}

// states counts the times a download has moved between running and finished.
// The Downloads tab is ordered by that, and the list of downloads itself does
// not change when one of them finishes.
func (dm *DownloadManager) states() binding.Int {
	dm.statesOnce.Do(func() { dm.statesBinding = binding.NewInt() })
	return dm.statesBinding
}

// noteStateChange tells whoever is listening that a download has moved.
func (dm *DownloadManager) noteStateChange() {
	states := dm.states()
	moves, _ := states.Get()
	_ = states.Set(moves + 1)
}

type queuedDownload struct {
	authService  *auth.Service
	game         db.Game
	downloadPath string
	language     string
	platformName string
	// languages and platforms are every choice that was ticked; the singular
	// fields above carry the first of each for whatever still expects one.
	// Empty slices mean the singular fields are the whole answer.
	languages       []string
	platforms       []string
	extrasFlag      bool
	dlcFlag         bool
	resumeFlag      bool
	flattenFlag     bool
	skipPatchesFlag bool
	keepLatestFlag  bool
	rommLayoutFlag  bool
	numThreads      int
}

func NewDownloadManager() *DownloadManager {
	a := fyne.CurrentApp()
	historyURI, err := storage.Child(a.Storage().RootURI(), "download_history.json")
	if err != nil {
		log.Error().Err(err).Msg("Failed to create history file path")
	}

	dm := &DownloadManager{
		Tasks:       binding.NewUntypedList(),
		historyPath: historyURI,
	}

	dm.loadHistory()
	return dm
}

// AddTask registers a download. The manager's lock is not held while the list
// is appended to: appending tells the UI, and the UI asks the manager what it
// is holding, which would be waiting on a lock the same call already has.
func (dm *DownloadManager) AddTask(task *DownloadTask) error {
	task.onStateChange = dm.noteStateChange
	return dm.Tasks.Append(task)
}

// canRetry reports whether this download can be started again. Tasks restored
// from the history file carry no request, so there is nothing to repeat.
func (t *DownloadTask) canRetry() bool {
	if t.request.game.ID == 0 {
		return false
	}
	state := t.State()
	return state == StateError || state == StateCancelled
}

// pause stops a running download, keeping what has arrived for the resume.
func (t *DownloadTask) pause() {
	if t.CancelFunc == nil {
		return
	}
	t.pausing.Store(true)
	t.CancelFunc()
}

// canResume reports whether a paused download can pick up where it stopped.
// One restored from the history file cannot: its request did not survive.
func (t *DownloadTask) canResume() bool {
	return t.State() == StatePaused && t.request.game.ID != 0
}

// resumePaused starts a paused download again from what is already on disk,
// replacing its entry. Resume is forced on: that is what pausing promised.
func (dm *DownloadManager) resumePaused(task *DownloadTask) error {
	if !task.canResume() {
		return errors.New("this download cannot be resumed")
	}
	request := task.request
	request.resumeFlag = true
	if err := dm.QueueOrStart(request); err != nil {
		return err
	}
	dm.removeTask(task)
	return nil
}

// retry starts a failed or cancelled download again, replacing its entry.
func (dm *DownloadManager) retry(task *DownloadTask) error {
	if !task.canRetry() {
		return errors.New("this download cannot be retried")
	}
	if err := dm.QueueOrStart(task.request); err != nil {
		return err
	}
	dm.removeTask(task)
	return nil
}

// removeTask drops a task from the list. The list is told after the lock is
// let go, for the reason AddTask gives.
func (dm *DownloadManager) removeTask(task *DownloadTask) {
	dm.mu.Lock()
	all, _ := dm.Tasks.Get()
	kept := make([]interface{}, 0, len(all))
	for _, raw := range all {
		if raw.(*DownloadTask).InstanceID != task.InstanceID {
			kept = append(kept, raw)
		}
	}
	dm.mu.Unlock()
	_ = dm.Tasks.Set(kept)
	dm.PersistHistory()
}

func (dm *DownloadManager) loadHistory() {
	if dm.historyPath == nil {
		return
	}

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

		task := &DownloadTask{
			ID:           pTask.ID,
			InstanceID:   pTask.InstanceID,
			Title:        pTask.Title,
			Status:       status,
			Progress:     progress,
			DownloadPath: pTask.DownloadPath,
			Details:      binding.NewString(),
			FileStatus:   binding.NewString(),
			CancelFunc:   nil,
		}
		task.SetState(pTask.State)
		uiTasks = append(uiTasks, task)
	}
	_ = dm.Tasks.Set(uiTasks)
	log.Info().Int("count", len(uiTasks)).Msg("Download history loaded.")
}

// historyKept is how many finished downloads are remembered between runs. The
// history only ever grew, and a list with a thousand entries in it is a list
// nobody reads.
const historyKept = 100

func (dm *DownloadManager) PersistHistory() {
	if dm.historyPath == nil {
		return
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	allTasks, _ := dm.Tasks.Get()
	persistentTasks := make([]PersistentDownloadTask, 0)

	for _, taskRaw := range allTasks {
		task := taskRaw.(*DownloadTask)
		if state := task.State(); state == StateCompleted || state == StateCancelled ||
			state == StateError || state == StatePaused {
			status, _ := task.Status.Get()
			persistentTasks = append(persistentTasks, PersistentDownloadTask{
				ID:           task.ID,
				InstanceID:   task.InstanceID,
				State:        state,
				Title:        task.Title,
				StatusText:   status,
				DownloadPath: task.DownloadPath,
			})
		}
	}

	// Most recent first, so what is dropped is the oldest.
	sort.Slice(persistentTasks, func(i, j int) bool {
		return persistentTasks[j].InstanceID.Before(persistentTasks[i].InstanceID)
	})
	if len(persistentTasks) > historyKept {
		persistentTasks = persistentTasks[:historyKept]
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

// downloadRow is a card in the download list. It is a widget rather than a
// nest of containers so its parts are reached by name: navigating this card by
// index has broken twice when the layout changed.
type downloadRow struct {
	widget.BaseWidget

	title      *widget.Label
	pauseBtn   *iconButton
	actionBtn  *widget.Button
	clearBtn   *iconButton
	status     *widget.Label
	details    *widget.Label
	progress   *widget.ProgressBar
	fileStatus *widget.Label
	fileScroll *container.Scroll
}

// newDownloadRow builds an empty card for the download list.
func newDownloadRow() fyne.CanvasObject {
	row := &downloadRow{
		title:      widget.NewLabel("Game Title"),
		pauseBtn:   newIconButton(theme.MediaPauseIcon(), "Pause, keeping what has arrived", nil),
		actionBtn:  widget.NewButtonWithIcon("Action", theme.CancelIcon(), nil),
		clearBtn:   newIconButton(theme.DeleteIcon(), "Take this off the list", nil),
		status:     widget.NewLabel("Status"),
		details:    widget.NewLabel("Details"),
		progress:   widget.NewProgressBar(),
		fileStatus: widget.NewLabel(""),
	}

	row.title.TextStyle = fyne.TextStyle{Bold: true}
	row.title.Truncation = fyne.TextTruncateEllipsis
	row.status.Wrapping = fyne.TextWrapWord
	row.details.TextStyle = fyne.TextStyle{Italic: true}
	row.details.Wrapping = fyne.TextWrapWord
	row.fileStatus.TextStyle = fyne.TextStyle{Monospace: true}
	row.fileStatus.Wrapping = fyne.TextWrapOff
	row.fileStatus.Truncation = fyne.TextTruncateEllipsis

	// Every row in a list is given the height of this template, so it reserves
	// room for the longest file list a card can show. Measured rather than hard
	// coded, so it follows the font size chosen in Settings.
	probe := widget.NewLabel("Ag")
	probe.TextStyle = fyne.TextStyle{Monospace: true}
	row.fileScroll = container.NewVScroll(row.fileStatus)
	row.fileScroll.SetMinSize(fyne.NewSize(0, probe.MinSize().Height*float32(fileStatusLines+1)))

	row.ExtendBaseWidget(row)
	return row
}

func (r *downloadRow) CreateRenderer() fyne.WidgetRenderer {
	topRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(r.pauseBtn, r.actionBtn, r.clearBtn), r.title)

	content := container.NewVBox(
		topRow,
		widget.NewSeparator(),
		r.status,
		r.details,
		r.progress,
		r.fileScroll,
	)

	card := widget.NewCard("", "", container.NewPadded(content))
	return widget.NewSimpleRenderer(container.NewVBox(card, widget.NewSeparator()))
}

// stillGoing reports whether a download has yet to finish, one way or another.
func stillGoing(task *DownloadTask) bool {
	switch task.State() {
	case StatePreparing, StateDownloading:
		return true
	default:
		return false
	}
}

// rowExpanded reports whether a download still has transfers to show. Finished
// ones have no file list and no speed, so their card can be much shorter.
func rowExpanded(task *DownloadTask) bool { return stillGoing(task) }

// tasksSnapshot is what the manager is holding, as downloads rather than as
// anonymous list items.
func (dm *DownloadManager) tasksSnapshot() []*DownloadTask {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	all, _ := dm.Tasks.Get()
	tasks := make([]*DownloadTask, 0, len(all))
	for _, raw := range all {
		if task, ok := raw.(*DownloadTask); ok {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

// orderedTasks puts the downloads in the order they matter: the ones still
// going first, in the order they started, then the finished ones with the most
// recent at the top. Listed in the order they were added, a long history sat
// above whatever was happening now.
func orderedTasks(tasks []*DownloadTask) []*DownloadTask {
	ordered := make([]*DownloadTask, len(tasks))
	copy(ordered, tasks)

	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if stillGoing(left) != stillGoing(right) {
			return stillGoing(left)
		}
		if stillGoing(left) {
			return left.InstanceID.Before(right.InstanceID)
		}
		return right.InstanceID.Before(left.InstanceID)
	})
	return ordered
}

// setRowExpanded shows or hides the parts only a running download needs.
func setRowExpanded(obj fyne.CanvasObject, expanded bool) {
	row, ok := obj.(*downloadRow)
	if !ok {
		return
	}
	if expanded {
		row.details.Show()
		row.fileScroll.Show()
	} else {
		row.details.Hide()
		row.fileScroll.Hide()
	}
	row.Refresh()
}

// downloadRowHeights measures the two card sizes the list uses.
func downloadRowHeights() (compact, full float32) {
	row := newDownloadRow()
	full = row.MinSize().Height
	setRowExpanded(row, false)
	compact = row.MinSize().Height
	return compact, full
}

func DownloadsTabUI(win fyne.Window, dm *DownloadManager) fyne.CanvasObject {
	compactHeight, fullHeight := downloadRowHeights()

	// The list is drawn from a snapshot, because the order downloads matter in
	// is not the order they were added in.
	var shown []*DownloadTask

	var list *widget.List
	list = widget.NewList(
		func() int { return len(shown) },
		newDownloadRow,
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(shown) {
				return
			}
			task := shown[id]

			// A finished download needs neither a speed nor a file list, so its
			// card is given only the room it uses.
			expanded := rowExpanded(task)
			setRowExpanded(obj, expanded)
			if expanded {
				list.SetItemHeight(id, fullHeight)
			} else {
				list.SetItemHeight(id, compactHeight)
			}

			row, ok := obj.(*downloadRow)
			if !ok {
				return
			}
			title, status, details := row.title, row.status, row.details
			progress, fileStatus := row.progress, row.fileStatus
			actionBtn, clearBtn := row.actionBtn, row.clearBtn

			title.SetText(task.Title)
			status.Bind(task.Status)
			details.Bind(task.Details)
			progress.Bind(task.Progress)
			fileStatus.Bind(task.FileStatus)

			clearBtn.OnTapped = func() { dm.removeTask(task) }

			// Only a transfer that is truly moving can be paused: a queued
			// placeholder has nothing on disk worth keeping yet.
			queuedStatus, _ := task.Status.Get()
			if stillGoing(task) && queuedStatus != "Queued" && task.CancelFunc != nil {
				row.pauseBtn.OnTapped = func() { task.pause() }
				row.pauseBtn.Show()
			} else {
				row.pauseBtn.Hide()
			}

			switch task.State() {
			case StateCompleted:
				actionBtn.SetIcon(theme.FolderOpenIcon())
				actionBtn.SetText("Open Folder")
				actionBtn.OnTapped = func() { openFolder(task.DownloadPath) }
				actionBtn.Enable()
				clearBtn.Show()
			case StatePaused:
				clearBtn.Show()
				if task.canResume() {
					actionBtn.SetIcon(theme.MediaPlayIcon())
					actionBtn.SetText("Resume")
					actionBtn.OnTapped = func() {
						if err := dm.resumePaused(task); err != nil {
							log.Error().Err(err).Str("game", task.Title).Msg("Failed to resume download")
						}
					}
					actionBtn.Enable()
					break
				}
				// Paused in an earlier run: the request did not survive, so
				// the way on is downloading the game again with resume on.
				actionBtn.SetIcon(theme.MediaPauseIcon())
				actionBtn.SetText("Paused")
				actionBtn.OnTapped = nil
				actionBtn.Disable()
			case StateCancelled, StateError:
				clearBtn.Show()
				if task.canRetry() {
					actionBtn.SetIcon(theme.ViewRefreshIcon())
					actionBtn.SetText("Retry")
					actionBtn.OnTapped = func() {
						if err := dm.retry(task); err != nil {
							log.Error().Err(err).Str("game", task.Title).Msg("Failed to retry download")
						}
					}
					actionBtn.Enable()
					break
				}
				// Nothing to repeat, so the button just states where it ended up.
				actionBtn.SetIcon(theme.ErrorIcon())
				actionBtn.SetText("Error")
				if task.State() == StateCancelled {
					actionBtn.SetIcon(theme.CancelIcon())
					actionBtn.SetText("Cancelled")
				}
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
				clearBtn.Hide()
			}
		},
	)

	// Nothing to show is worth saying: a blank page reads as something that has
	// not loaded.
	empty := emptyState(theme.DownloadIcon(), "No downloads yet",
		"What you download from the catalogue shows its progress here.", nil)
	body := container.NewStack()

	relist := func() {
		shown = orderedTasks(dm.tasksSnapshot())
		if len(shown) == 0 {
			body.Objects = []fyne.CanvasObject{empty}
		} else {
			body.Objects = []fyne.CanvasObject{list}
		}
		body.Refresh()
		list.Refresh()
	}
	relist()
	dm.Tasks.AddListener(binding.NewDataListener(relist))
	dm.states().AddListener(binding.NewDataListener(relist))

	totalsLabel := widget.NewLabelWithData(dm.totals())
	totalsLabel.TextStyle = fyne.TextStyle{Bold: true}
	header := container.NewPadded(totalsLabel)
	dm.refreshTotals()

	clearFinished := func() {
		dm.mu.Lock()
		currentTasks, _ := dm.Tasks.Get()
		keptTasks := make([]interface{}, 0)
		for _, taskRaw := range currentTasks {
			task := taskRaw.(*DownloadTask)
			if state := task.State(); state != StateCompleted && state != StateCancelled && state != StateError {
				keptTasks = append(keptTasks, task)
			}
		}
		dm.mu.Unlock()
		// Told after letting go of the lock, for the reason AddTask gives.
		_ = dm.Tasks.Set(keptTasks)
		dm.PersistHistory()
		dm.refreshTotals()
	}
	clearAllBtn := widget.NewButton("Clear All Finished", func() {
		// Paused downloads are not finished: someone means to come back.
		finished := 0
		for _, task := range dm.tasksSnapshot() {
			switch task.State() {
			case StateCompleted, StateCancelled, StateError:
				finished++
			}
		}
		if finished == 0 {
			return
		}
		// The history is also the record of where downloads went, so taking all
		// of it is asked about rather than done.
		dialog.ShowConfirm("Clear All Finished",
			fmt.Sprintf("Remove %d finished %s from the list?", finished, downloadsWord(finished)),
			func(confirmed bool) {
				if confirmed {
					clearFinished()
				}
			}, win)
	})
	bottomBar := container.NewHBox(layout.NewSpacer(), clearAllBtn)

	return container.NewBorder(header, bottomBar, nil, nil, body)
}

// runningTaskFor is the download on its way for a game, nil when there is none.
// Queued downloads count: to the person looking at the list, waiting to start
// is a download on its way. Safe to call on a nil manager.
func (dm *DownloadManager) runningTaskFor(gameID int) *DownloadTask {
	if dm == nil {
		return nil
	}
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	all, _ := dm.Tasks.Get()
	for _, raw := range all {
		if task, ok := raw.(*DownloadTask); ok && task.ID == gameID && stillGoing(task) {
			return task
		}
	}
	return nil
}

// inFlightCount is how many downloads are running or waiting in the queue: the
// number a person glancing at the Downloads tab wants to know.
func (dm *DownloadManager) inFlightCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	all, _ := dm.Tasks.Get()
	count := 0
	for _, raw := range all {
		if task, ok := raw.(*DownloadTask); ok && stillGoing(task) {
			count++
		}
	}
	return count
}

func (dm *DownloadManager) activeCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	all, _ := dm.Tasks.Get()
	c := 0
	for _, tRaw := range all {
		t := tRaw.(*DownloadTask)
		switch t.State() {
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

const (
	prefMaxConcurrent    = "download.maxConcurrent"
	defaultMaxConcurrent = 2
)

// maxConcurrentDownloads reads the configured limit. Older versions stored it
// as a string, so that form is still accepted.
func maxConcurrentDownloads(prefs fyne.Preferences) int {
	if v := prefs.IntWithFallback(prefMaxConcurrent, 0); v > 0 {
		return v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(prefs.String(prefMaxConcurrent))); err == nil && v > 0 {
		return v
	}
	return defaultMaxConcurrent
}

func (dm *DownloadManager) maxConcurrent() int {
	return maxConcurrentDownloads(fyne.CurrentApp().Preferences())
}

// cancelQueued drops a download that has not started yet from the queue.
func (dm *DownloadManager) cancelQueued(task *DownloadTask) {
	dm.mu.Lock()
	kept := make([]queuedDownload, 0, len(dm.queue))
	removed := false
	for _, q := range dm.queue {
		if !removed && q.game.ID == task.ID {
			removed = true
			continue
		}
		kept = append(kept, q)
	}
	dm.queue = kept
	dm.mu.Unlock()

	task.SetState(StateCancelled)
	_ = task.Status.Set("Cancelled")
	dm.PersistHistory()
	// Taking the last waiting download out closes the batch out too.
	announceIfLast(dm, StateCancelled, task.Title)
}

func (dm *DownloadManager) QueueOrStart(q queuedDownload) error {
	// Prevent duplicate active or queued
	dm.mu.RLock()
	all, _ := dm.Tasks.Get()
	for _, tRaw := range all {
		t := tRaw.(*DownloadTask)
		if state := t.State(); t.ID == q.game.ID && (state == StatePreparing || state == StateDownloading) {
			dm.mu.RUnlock()
			return ErrDownloadInProgress
		}
	}
	dm.mu.RUnlock()
	if dm.activeCount() < dm.maxConcurrent() {
		return executeDownload(dm, q)
	}
	// Enqueue
	dm.mu.Lock()
	dm.queue = append(dm.queue, q)
	// Add placeholder task
	placeholder := &DownloadTask{
		ID:         q.game.ID,
		InstanceID: time.Now(),
		Title:      q.game.Title,
		Status:     binding.NewString(),
		Details:    binding.NewString(),
		Progress:   binding.NewFloat(),
		FileStatus: binding.NewString(),
		// Kept so a download cancelled while it was still waiting can be
		// started again, the same as one cancelled after it began.
		request: q,
	}
	placeholder.SetState(StatePreparing)
	_ = placeholder.Status.Set("Queued")
	// The Downloads tab offers a Cancel button for this task, so it needs a way
	// to take the download back out of the queue.
	placeholder.CancelFunc = func() { dm.cancelQueued(placeholder) }
	dm.mu.Unlock()

	// Appended after letting go of the lock, for the reason AddTask gives.
	return dm.AddTask(placeholder)
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
			if t.ID == next.game.ID && t.State() == StatePreparing {
				status, _ := t.Status.Get()
				if status == "Queued" {
					continue
				}
			}
			filtered = append(filtered, tRaw)
		}
		dm.mu.Unlock()
		// Told after letting go of the lock, for the reason AddTask gives.
		_ = dm.Tasks.Set(filtered)
		_ = executeDownload(dm, next)
	}
}
