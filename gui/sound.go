package gui

import (
	"bytes"
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
	speakerOnce sync.Once
	mixer       *beep.Mixer
	sampleRate  beep.SampleRate
)

func initSpeaker(sr beep.SampleRate) {
	speakerOnce.Do(func() {
		sampleRate = sr
		// The buffer size should be large enough to avoid under-runs.
		bufferSize := sr.N(time.Second / 10)
		if err := speaker.Init(sampleRate, bufferSize); err != nil {
			log.Error().Err(err).Msg("Failed to initialize speaker")
			return
		}
		mixer = &beep.Mixer{}
		speaker.Play(mixer)
	})
}

func PlayNotificationSound() {
	a := fyne.CurrentApp()
	if !a.Preferences().BoolWithFallback("soundEnabled", true) {
		return
	}

	filePath := a.Preferences().String("soundFilePath")
	var reader io.ReadCloser
	isDefault := false

	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			log.Error().Err(err).Str("path", filePath).Msg("Failed to open custom sound file, falling back to default")
			isDefault = true
		} else {
			reader = f
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
		filePath = ".mp3" // Pretend it's an mp3 for the decoder switch
	}
	defer reader.Close()

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
		log.Error().Err(err).Msg("Failed to decode audio stream")
		return
	}

	// Initialize the speaker with the format of the first sound played.
	initSpeaker(format.SampleRate)

	// Create a new streamer that is resampled to the mixer's sample rate.
	resampled := beep.Resample(4, format.SampleRate, sampleRate, streamer)

	// Add the resampled audio to the mixer. The mixer handles playing it.
	done := make(chan bool)
	mixer.Add(beep.Seq(resampled, beep.Callback(func() {
		done <- true
	})))

	// Wait for this specific sound to finish.
	<-done
}
