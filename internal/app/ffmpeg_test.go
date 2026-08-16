package app

import (
	"runtime"
	"strings"
	"testing"
)

func TestFFmpegInstallHint(t *testing.T) {
	h := FFmpegInstallHint()
	for _, want := range []string{
		"https://github.com/Lamprozx/ffmpeg-minimal",
		"ffmpeg-min-arm64",
		"ffmpeg-min-linux-amd64",
		"$PREFIX/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("hint missing %q:\n%s", want, h)
		}
	}
}

func TestMinimalFFmpegTargetLinuxAmd64(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("linux/amd64 only")
	}
	url, path, ok := minimalFFmpegTarget()
	if !ok {
		t.Fatal("expected a target on linux/amd64")
	}
	if !strings.Contains(url, "ffmpeg-min-linux-amd64") {
		t.Errorf("url = %q", url)
	}
	if !strings.HasSuffix(path, "ffmpeg") {
		t.Errorf("install path = %q", path)
	}
}

func TestMinimalFFmpegTargetAndroidEnv(t *testing.T) {
	t.Setenv("ANDROID_ROOT", "/system")
	t.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	url, path, ok := minimalFFmpegTarget()
	if !ok {
		t.Fatal("expected a target when ANDROID_ROOT is set")
	}
	if !strings.Contains(url, "ffmpeg-min-arm64") {
		t.Errorf("url = %q", url)
	}
	if want := "/data/data/com.termux/files/usr/bin/ffmpeg"; path != want {
		t.Errorf("install path = %q, want %q", path, want)
	}
}
