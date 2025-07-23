package gui

import (
	"bytes"
	_ "embed"
	"io"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/rs/zerolog/log"
)

//go:embed assets/ding-small-bell-sfx-233008.mp3
var defaultDingSound []byte

var speakerOnce sync.Once

func PlayNotificationSound() {
	a := fyne.CurrentApp()
	if !a.Preferences().BoolWithFallback("soundEnabled", true) {
		return
	}

	filePath := a.Preferences().String("soundFilePath")
	var reader io.ReadCloser
	var err error

	if filePath != "" {
		reader, err = os.Open(filePath)
		if err != nil {
			log.Error().Err(err).Str("path", filePath).Msg("Failed to open custom sound file, falling back to default")
			reader = io.NopCloser(bytes.NewReader(defaultDingSound))
		}
	} else {
		if len(defaultDingSound) == 0 {
			log.Warn().Msg("No custom sound set and default sound asset is missing.")
			return
		}
		reader = io.NopCloser(bytes.NewReader(defaultDingSound))
	}
	defer reader.Close()

	streamer, format, err := mp3.Decode(reader)
	if err != nil {
		log.Error().Err(err).Msg("Failed to decode mp3 stream")
		return
	}
	defer streamer.Close()

	speakerOnce.Do(func() {
		if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)); err != nil {
			log.Error().Err(err).Msg("Failed to initialize speaker")
		}
	})

	done := make(chan bool)
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		done <- true
	})))

	<-done
}
