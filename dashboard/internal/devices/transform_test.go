package devices

import "testing"

// TestBuildTransformSource covers the rotate/flip → ffmpeg "-vf" mapping, the
// merge-vs-wrap behaviour for the different base source forms, and the cases that
// must error rather than silently corrupt the pipeline.
func TestBuildTransformSource(t *testing.T) {
	const rtsp = "rtsp://cam/stream"
	cases := []struct {
		name         string
		base         string
		rotate       int
		flipH, flipV bool
		hardware     bool
		want         string
		wantErr      bool
	}{
		// No-op: leave the base pristine.
		{name: "noop rtsp", base: rtsp, want: rtsp},
		{name: "noop ffmpeg", base: "ffmpeg:" + rtsp + "#video=h264", want: "ffmpeg:" + rtsp + "#video=h264"},

		// Rotations on a bare RTSP base.
		{name: "rot90", base: rtsp, rotate: 90, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf transpose=1"},
		{name: "rot180", base: rtsp, rotate: 180, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf transpose=2,transpose=2"},
		{name: "rot270", base: rtsp, rotate: 270, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf transpose=2"},

		// Flips.
		{name: "hflip", base: rtsp, flipH: true, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf hflip"},
		{name: "vflip", base: rtsp, flipV: true, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf vflip"},
		{name: "hvflip", base: rtsp, flipH: true, flipV: true, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf hflip,vflip"},

		// Rotate + flip combined: rotate filter(s) first, then flips.
		{name: "rot90+hflip", base: rtsp, rotate: 90, flipH: true, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf transpose=1,hflip"},

		// Hardware flag appends #hardware once.
		{name: "rot90 hw", base: rtsp, rotate: 90, hardware: true, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf transpose=1#hardware"},

		// Existing ffmpeg base: merge params, don't nest or duplicate video=h264.
		{name: "ffmpeg base rot90", base: "ffmpeg:" + rtsp + "#video=h264", rotate: 90, want: "ffmpeg:" + rtsp + "#video=h264#raw=-vf transpose=1"},
		// Existing ffmpeg base already has #hardware: don't duplicate it.
		{name: "ffmpeg base hw rot90", base: "ffmpeg:" + rtsp + "#video=h264#hardware", rotate: 90, hardware: true, want: "ffmpeg:" + rtsp + "#video=h264#hardware#raw=-vf transpose=1"},

		// Error: exec: can't be transcoded.
		{name: "exec err", base: "exec:ffmpeg -i x", rotate: 90, wantErr: true},
		// Error: a source that already carries a raw/-vf filter would conflict.
		{name: "existing raw err", base: "ffmpeg:" + rtsp + "#video=h264#raw=-vf eq=brightness=0.1", rotate: 90, wantErr: true},
		{name: "existing rotate err", base: "ffmpeg:" + rtsp + "#video=h264#rotate=90", flipH: true, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildTransformSource(tc.base, tc.rotate, tc.flipH, tc.flipV, tc.hardware)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestBuildTransformSourcePreservesCredentials confirms a transform wraps a source
// whose URL already carries userinfo (injected by Source()) without mangling it.
func TestBuildTransformSourcePreservesCredentials(t *testing.T) {
	rec := CameraRecord{Src: "rtsp://cam/stream", Username: "bob", Password: "secret", Rotate: 90}
	got, err := BuildTransformSource(rec.Source(), rec.Rotate, rec.FlipH, rec.FlipV, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "ffmpeg:rtsp://bob:secret@cam/stream#video=h264#raw=-vf transpose=1"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
