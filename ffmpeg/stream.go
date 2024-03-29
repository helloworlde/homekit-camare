package ffmpeg

import (
	"fmt"
	"github.com/brutella/hap/log"
	"github.com/brutella/hap/rtp"
	"os/exec"
	"strings"
	"syscall"
)

type stream struct {
	inputDevice     string
	inputFilename   string
	h264Decoder     string
	h264Encoder     string
	minVideoBitrate int

	req  rtp.SetupEndpoints
	resp rtp.SetupEndpointsResponse

	cmd *exec.Cmd
}

func (s *stream) isActive() bool {
	return s.cmd != nil
}

func (s *stream) stop() {
	log.Debug.Println("stop stream")

	if s.cmd != nil {
		s.cmd.Process.Signal(syscall.SIGINT)
		s.cmd.Wait()
		s.cmd = nil
	}
}

func (s *stream) start(video rtp.VideoParameters, audio rtp.AudioParameters) error {
	log.Debug.Println("start stream")

	// -vsync 2: Fixes "Frame rate very high for a muxer not efficiently supporting it."
	// -framerate before -i specifies the framerate for the input, after -i sets it for the output https://stackoverflow.com/questions/38498599/webcam-with-ffmpeg-on-mac-selected-framerate-29-970030-is-not-supported-by-th#38549528
	ffmpegVideo := fmt.Sprintf("-re -i %s", s.inputFilename) +
		// 使用软件解码
		fmt.Sprintf(" -allow_sw 1") +
		fmt.Sprintf(" -framerate %d", s.framerate(video.Attributes)) +
		fmt.Sprintf("%s", s.videoDecoderOption(video)) +
		//fmt.Sprintf(" -re -i %s", s.inputFilename) +
		" -an" +
		fmt.Sprintf(" -codec:v %s", s.videoEncoder(video)) +
		" -pix_fmt yuv420p -vsync vfr" +
		// 5帧每秒
		fmt.Sprintf(" -r 10") +

		// height "-2" keeps the aspect ratio
		fmt.Sprintf(" -video_size %d:-2", video.Attributes.Width) +
		fmt.Sprintf(" -framerate %d", video.Attributes.Framerate) +

		// 2019-06-20 (mah)
		//   Specifying profiles in h264_omx was added in ffmpeg 3.3
		//   https://github.com/FFmpeg/FFmpeg/commit/13332504c98918447159da2a1a34e377dca360e2#diff-36301d4a4bc7200caee9fbe8e8d8cc20
		//   hkcam currently uses ffmpeg 3.2
		// 2018-08-18 (mah)
		//   Disable profile arguments because it cannot be parsed
		// [h264_omx @ 0x93a410] [Eval @ 0xbeaad160] Undefined constant or missing '(' in 'high'
		// fmt.Sprintf(" -profile:v %s", videoProfile(video.CodecParams)) +
		fmt.Sprintf(" -level:v %s", videoLevel(video.CodecParams)) +
		" -f rawvideo" +
		fmt.Sprintf(" -b:v %dk", s.videoBitrate(video)) +
		fmt.Sprintf(" -payload_type %d", video.RTP.PayloadType) +
		fmt.Sprintf(" -ssrc %d", s.resp.SsrcVideo) +
		" -f rtp -srtp_out_suite AES_CM_128_HMAC_SHA1_80" +
		fmt.Sprintf(" -srtp_out_params %s", s.req.Video.SrtpKey()) +
		fmt.Sprintf(" srtp://%s:%d?rtcpport=%d&pkt_size=%s&timeout=60", s.req.ControllerAddr.IPAddr, s.req.ControllerAddr.VideoRtpPort, s.req.ControllerAddr.VideoRtpPort, videoMTU(s.req))

	log.Info.Println("ffmpeg 命令: ffmpeg", ffmpegVideo)
	args := strings.Split(ffmpegVideo, " ")
	cmd := exec.Command("ffmpeg", args[:]...)
	cmd.Stdout = Stdout
	cmd.Stderr = Stderr

	log.Debug.Println(cmd)

	err := cmd.Start()
	if err == nil {
		s.cmd = cmd
	}

	return err
}

// TODO (mah) test
func (s *stream) suspend() {
	log.Debug.Println("suspend stream")
	s.cmd.Process.Signal(syscall.SIGSTOP)
}

// TODO (mah) test
func (s *stream) resume() {
	log.Debug.Println("resume stream")
	s.cmd.Process.Signal(syscall.SIGCONT)
}

// TODO (mah) implement
func (s *stream) reconfigure(video rtp.VideoParameters, audio rtp.AudioParameters) error {
	if s.cmd != nil {
		log.Debug.Printf("reconfigure() is not implemented %+v %+v\n", video, audio)
	}

	return nil
}

func (s *stream) videoEncoder(param rtp.VideoParameters) string {
	switch param.CodecType {
	case rtp.VideoCodecType_H264:
		return s.h264Encoder
	}

	return "?"
}

func (s *stream) videoDecoderOption(param rtp.VideoParameters) string {
	switch param.CodecType {
	case rtp.VideoCodecType_H264:
		if s.h264Decoder != "" {
			return fmt.Sprintf(" -codec:v %s", s.h264Decoder)
		}
	}

	return ""
}

func (s *stream) videoBitrate(param rtp.VideoParameters) int {
	br := int(param.RTP.Bitrate)
	if s.minVideoBitrate > br {
		br = s.minVideoBitrate
	}

	return br
}

func (s *stream) framerate(attr rtp.VideoCodecAttributes) byte {
	if s.inputDevice == "avfoundation" {
		// avfoundation only supports 30 fps on a MacBook Pro (Retina, 15-inch, Late 2013) running macOS 10.12 Sierra
		// TODO (mah) test this with other Macs
		return 30
	}

	return attr.Framerate
}

// https://superuser.com/a/564007
func videoLevel(param rtp.VideoCodecParameters) string {
	for _, l := range param.Levels {
		switch l.Level {
		case rtp.VideoCodecLevel3_1:
			return "3.1"
		case rtp.VideoCodecLevel3_2:
			return "3.2"
		case rtp.VideoCodecLevel4:
			return "4.0"
		default:
			break
		}
	}

	return ""
}

func videoMTU(setup rtp.SetupEndpoints) string {
	switch setup.ControllerAddr.IPVersion {
	case rtp.IPAddrVersionv4:
		return "1378"
	case rtp.IPAddrVersionv6:
		return "1228"
	}

	return "1378"
}
