package camera

import "testing"

func TestCheckUserSource(t *testing.T) {
	bad := []string{
		"exec:touch /tmp/x",
		"ffmpeg:/etc/passwd#video=h264",
		"ffmpeg:http://x -i /etc/passwd#video=h264",
		"ffmpeg:rtsp://x#raw=-i /etc/passwd -f mpegts",
		"ffmpeg:rtsp://x#video=h264#raw=-f segment /tmp/a",
		"ffmpeg:rtsp://x#raw=-vf movie=/etc/passwd",
		"ffmpeg:rtsp://x#input=-re -i foo",
		"ffmpeg:rtsp://x#global=-y",
		"ffmpeg:rtsp://x#raw=-vf scale=1:1 -filter_complex x",
	}
	for _, s := range bad {
		if checkUserSource(s) == nil {
			t.Errorf("checkUserSource(%q) = nil, want error", s)
		}
	}
	good := []string{
		"rtsp://user:pw@10.0.0.2:554/stream",
		"ffmpeg:http://10.0.0.5/cam.h264#video=h264",
		"ffmpeg:http://10.0.0.5/cam#video=h264#hardware",
		"ffmpeg:rtsp://x#video=h264#raw=-r 15 -b:v 2M",
		"ffmpeg:rtsp://x#input=rtsp/udp",
		"onvif://user:pw@10.0.0.2",
	}
	for _, s := range good {
		if err := checkUserSource(s); err != nil {
			t.Errorf("checkUserSource(%q) = %v, want nil", s, err)
		}
	}
}
