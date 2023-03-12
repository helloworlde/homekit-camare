package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/log"
	"github.com/brutella/hkcam"
	"github.com/brutella/hkcam/ffmpeg"
	"image"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

var (
	// Date is build date.
	Date string

	// Version is the app version.
	Version string

	// BuildMode is the build mode ("debug", "release")
	BuildMode string
)

func main() {

	// Platform dependent flags
	var inputDevice *string
	var inputFilename *string
	var loopbackFilename *string
	var h264Encoder *string
	var h264Decoder *string

	if runtime.GOOS == "linux" {
		inputDevice = flag.String("input_device", "v4l2", "video input device")
		inputFilename = flag.String("input_filename", "/dev/video0", "video input device filename")
		loopbackFilename = flag.String("loopback_filename", "/dev/video99", "video loopback device filename")
		h264Decoder = flag.String("h264_decoder", "", "h264 video decoder")
		h264Encoder = flag.String("h264_encoder", "h264_v4l2m2m", "h264 video encoder")
	} else if runtime.GOOS == "darwin" { // macOS
		inputDevice = flag.String("input_device", "avfoundation", "video input device")
		inputFilename = flag.String("input_filename", "default", "video input device filename")
		// loopback is not needed on macOS because avfoundation provides multi-access to the camera
		loopbackFilename = flag.String("loopback_filename", "", "video loopback device filename")
		h264Decoder = flag.String("h264_decoder", "", "h264 video decoder")
		h264Encoder = flag.String("h264_encoder", "h264_videotoolbox", "h264 video encoder")
	} else {
		log.Info.Fatalf("%s platform is not supported", runtime.GOOS)
	}

	var minVideoBitrate *int = flag.Int("min_video_bitrate", 0, "minimum video bit rate in kbps")
	var multiStream *bool = flag.Bool("multi_stream", false, "Allow multiple clients to view the stream simultaneously")
	var dataDir *string = flag.String("data_dir", "db", "Path to data directory")
	var verbose *bool = flag.Bool("verbose", false, "Verbose logging")
	var pin *string = flag.String("pin", "00102003", "PIN for HomeKit pairing")
	var port *string = flag.String("port", "", "Port on which transport is reachable")

	flag.Parse()

	if *verbose {
		log.Debug.Enable()
		ffmpeg.EnableVerboseLogging()
	}

	log.Info.Printf("version %s (built at %s)\n", Version, Date)

	switchInfo := accessory.Info{Name: "Camera", Firmware: Version, Manufacturer: "Matthias Hochgatterer"}
	cam := accessory.NewCamera(switchInfo)

	cfg := ffmpeg.Config{
		InputDevice:      *inputDevice,
		InputFilename:    *inputFilename,
		LoopbackFilename: *loopbackFilename,
		H264Decoder:      *h264Decoder,
		H264Encoder:      *h264Encoder,
		MinVideoBitrate:  *minVideoBitrate,
		MultiStream:      *multiStream,
	}

	ffmpeg := hkcam.SetupFFMPEGStreaming(cam, cfg)

	// Add a custom camera control service to record snapshots
	cc := hkcam.NewCameraControl()
	cam.Control.AddC(cc.Assets.C)
	cam.Control.AddC(cc.GetAsset.C)
	cam.Control.AddC(cc.DeleteAssets.C)
	cam.Control.AddC(cc.TakeSnapshot.C)

	store := hap.NewFsStore(*dataDir)
	s, err := hap.NewServer(store, cam.A)
	if err != nil {
		log.Info.Panic(err)
	}

	s.Pin = *pin
	s.Addr = fmt.Sprintf(":%s", *port)

	cc.SetupWithDir(*dataDir)
	cc.CameraSnapshotReq = func(width, height uint) (*image.Image, error) {
		snapshot, err := ffmpeg.Snapshot(width, height)
		if err != nil {
			return nil, err
		}

		return &snapshot.Image, nil
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	signal.Notify(c, syscall.SIGUSR1)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		signal.Stop(c) // stop delivering signals
		cancel()
	}()

	if err := s.ListenAndServe(ctx); err != nil {
		if err != ctx.Err() {
			log.Info.Println(err)
		}
	}
}
