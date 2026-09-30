// This example generates a Wan2.2 S2V video from a source image and a WAV
// speech track. It writes Motion-JPEG video and PCM WAV audio as separate
// files because model.SaveAVI does not mux audio.
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries and a Wan2.2 S2V 14B model bundle.
//
// Run the example like this from the root of the project:
// $ make example-malina-s2v
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	buckyaudio "github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"golang.org/x/image/draw"
)

var modelSource = models.BundleWan22S2V14B

const (
	imageFile  = "samples/adetailer-face.png"
	audioFile  = "samples/jfk.wav"
	outputFile = "malina-s2v.avi"
	prompt     = "a person speaking naturally to the camera"
	width      = 832
	height     = 480
)

func main() {
	if err := run(); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func run() error {
	manifest, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	source, err := loadImage(imageFile)
	if err != nil {
		return err
	}
	source = resize(source, width, height)

	drivingAudio, err := loadAudio(audioFile)
	if err != nil {
		return err
	}

	mln, err := malina.NewWithContext(
		ctx,
		model.WithDiffusionModelPath(manifest.Files[string(models.RoleDiffusion)]),
		model.WithVAEPath(manifest.Files[string(models.RoleVAE)]),
		model.WithT5XXLPath(manifest.Files[string(models.RoleT5XXL)]),
		model.WithAudioEncoderPath(manifest.Files[string(models.RoleAudioEncoder)]),
		model.WithDiffusionFlashAttention(true),
	)
	if err != nil {
		return fmt.Errorf("load Wan2.2 S2V: %w", err)
	}

	defer func() {
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Println("unload:", err)
		}
	}()

	params := model.NewVideoParams()
	params.Prompt = prompt
	params.Width = width
	params.Height = height
	params.Steps = 20
	params.Frames = 81
	params.FPS = 16
	params.Seed = 42
	params.InitImage = source
	params.RefAudios = []model.Audio{drivingAudio}

	video, err := mln.GenerateVideo(ctx, params)
	if err != nil {
		return fmt.Errorf("generate video: %w", err)
	}
	if err := model.SaveAVI(outputFile, video.Frames, video.FPS, 90); err != nil {
		return err
	}

	outputAudio := drivingAudio
	if video.Audio != nil {
		outputAudio = *video.Audio
	}
	wavPath := strings.TrimSuffix(outputFile, filepath.Ext(outputFile)) + ".wav"
	if err := saveWAV(wavPath, outputAudio); err != nil {
		return err
	}

	fmt.Printf("Wrote %s and %s (%d frames at %d fps)\n", outputFile, wavPath, len(video.Frames), video.FPS)
	fmt.Printf("Mux with: ffmpeg -i %q -i %q -c:v copy -c:a aac -shortest malina-s2v-with-audio.mp4\n", outputFile, wavPath)

	return nil
}

func installSystem() (models.Manifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	lib, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return models.Manifest{}, err
	}
	if _, err := lib.Download(ctx, malina.FmtLogger); err != nil {
		return models.Manifest{}, fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(malina.WithLibPath(lib.LibsPath())); err != nil {
		return models.Manifest{}, fmt.Errorf("unable to init Malina: %w", err)
	}

	mdls, err := models.New()
	if err != nil {
		return models.Manifest{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model bundle:", modelSource)

	manifest, err := mdls.DownloadBundle(ctx, modelSource)
	if err != nil {
		return models.Manifest{}, fmt.Errorf("unable to install model bundle: %w", err)
	}

	return manifest, nil
}

func loadImage(filename string) (image.Image, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	defer file.Close()

	source, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	return source, nil
}

func resize(source image.Image, width int, height int) image.Image {
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), draw.Over, nil)
	return target
}

func loadAudio(filename string) (model.Audio, error) {
	file, err := os.Open(filename)
	if err != nil {
		return model.Audio{}, fmt.Errorf("open audio: %w", err)
	}
	defer file.Close()

	samples, sampleRate, channels, err := buckyaudio.DecodeWAV(file)
	if err != nil {
		return model.Audio{}, fmt.Errorf("decode WAV audio: %w", err)
	}

	return model.Audio{SampleRate: uint32(sampleRate), Channels: uint32(channels), Data: samples}, nil
}

func saveWAV(filename string, audio model.Audio) error {
	if audio.SampleRate == 0 || audio.Channels == 0 || len(audio.Data)%int(audio.Channels) != 0 {
		return errors.New("save WAV: invalid audio")
	}
	if audio.Channels > math.MaxUint16/2 || uint64(audio.SampleRate)*uint64(audio.Channels)*2 > math.MaxUint32 || uint64(len(audio.Data))*2 > math.MaxUint32-36 {
		return errors.New("save WAV: audio is too large")
	}

	dataSize := uint32(len(audio.Data) * 2)
	blockAlign := uint16(audio.Channels * 2)
	header := make([]byte, 44+dataSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(audio.Channels))
	binary.LittleEndian.PutUint32(header[24:28], audio.SampleRate)
	binary.LittleEndian.PutUint32(header[28:32], audio.SampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)

	for i, sample := range audio.Data {
		sample = min(max(sample, -1), 1)
		binary.LittleEndian.PutUint16(header[44+i*2:], uint16(int16(math.Round(float64(sample*32_767)))))
	}

	if err := os.WriteFile(filename, header, 0o644); err != nil {
		return fmt.Errorf("save WAV: %w", err)
	}

	return nil
}
