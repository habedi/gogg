package gui

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
	"github.com/rs/zerolog/log"
)

//go:embed assets/ding-small-bell-sfx-233008.mp3
var defaultDingSound []byte

var (
	speakerOnce     sync.Once
	mixer           *beep.Mixer
	sampleRate      beep.SampleRate
	currentSound    context.CancelFunc
	currentSoundMux sync.Mutex
	soundPlaying    bool
)

func initSpeaker(sr beep.SampleRate) {
	speakerOnce.Do(func() {
		sampleRate = sr
		bufferSize := sr.N(time.Second / 10)
		if err := speaker.Init(sampleRate, bufferSize); err != nil {
			log.Error().Err(err).Msg("Failed to initialize speaker")
			return
		}
		mixer = &beep.Mixer{}
		speaker.Play(mixer)
	})
}

func validateAudioFile(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("empty file path")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file")
	}

	if info.Size() == 0 {
		return fmt.Errorf("file is empty")
	}

	if info.Size() > 50*1024*1024 {
		return fmt.Errorf("file is too large (>50MB), please use a shorter audio clip")
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".mp3" && ext != ".wav" && ext != ".ogg" {
		return fmt.Errorf("unsupported file format: %s (supported: .mp3, .wav, .ogg)", ext)
	}

	return nil
}

func PlayNotificationSound() {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("Recovered from panic in PlayNotificationSound")
		}
	}()

	a := fyne.CurrentApp()
	if !a.Preferences().BoolWithFallback("soundEnabled", true) {
		return
	}

	currentSoundMux.Lock()
	if currentSound != nil && soundPlaying {
		currentSound()
	}
	ctx, cancel := context.WithCancel(context.Background())
	currentSound = cancel
	soundPlaying = true
	currentSoundMux.Unlock()

	defer func() {
		currentSoundMux.Lock()
		soundPlaying = false
		currentSound = nil
		currentSoundMux.Unlock()
	}()

	filePath := a.Preferences().String("soundFilePath")
	var reader io.ReadCloser
	isDefault := false

	if filePath != "" {
		if err := validateAudioFile(filePath); err != nil {
			log.Error().Err(err).Str("path", filePath).Msg("Invalid custom sound file, falling back to default")
			isDefault = true
		} else {
			f, err := os.Open(filePath)
			if err != nil {
				log.Error().Err(err).Str("path", filePath).Msg("Failed to open custom sound file, falling back to default")
				isDefault = true
			} else {
				reader = f
			}
		}
	} else {
		isDefault = true
	}

	if isDefault {
		if len(defaultDingSound) == 0 {
			log.Warn().Msg("No custom sound set and default sound asset is missing.")
			return
		}
		reader = io.NopCloser(bytes.NewReader(defaultDingSound))
		filePath = ".mp3"
	}
	defer func() {
		if err := reader.Close(); err != nil {
			log.Debug().Err(err).Msg("Failed to close audio reader")
		}
	}()

	var streamer beep.StreamSeekCloser
	var format beep.Format
	var err error

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".mp3":
		streamer, format, err = mp3.Decode(reader)
	case ".wav":
		streamer, format, err = wav.Decode(reader)
	case ".ogg":
		streamer, format, err = vorbis.Decode(reader)
	default:
		err = fmt.Errorf("unsupported sound format for file: %s", filePath)
	}

	if err != nil {
		log.Error().Err(err).Str("path", filePath).Msg("Failed to decode audio stream - file may be corrupted or invalid")
		return
	}
	defer func() {
		if err := streamer.Close(); err != nil {
			log.Debug().Err(err).Msg("Failed to close audio streamer")
		}
	}()

	initSpeaker(format.SampleRate)

	resampled := beep.Resample(4, format.SampleRate, sampleRate, streamer)

	done := make(chan bool, 1)
	mixer.Add(beep.Seq(resampled, beep.Callback(func() {
		select {
		case done <- true:
		default:
		}
	})))

	select {
	case <-done:
	case <-ctx.Done():
		log.Debug().Msg("Sound playback cancelled")
	case <-time.After(30 * time.Second):
		log.Warn().Msg("Sound playback timeout - audio file may be too long")
	}
}
