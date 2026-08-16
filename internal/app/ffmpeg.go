package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"opencall/internal/console"
)

const (
	ffmpegMinimalVersion = "v1.0.0"
	ffmpegMinimalRepo    = "https://github.com/Lamprozx/ffmpeg-minimal"
	ffmpegMinimalBase    = ffmpegMinimalRepo + "/releases/download/" + ffmpegMinimalVersion
)

var errFFmpegMissing = errors.New("ffmpeg is required but not installed")

// FFmpegAvailable reports whether the `ffmpeg` binary is on PATH.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// EnsureFFmpeg verifies that ffmpeg is available. When it is not, it explains
// why it is needed and offers to download and install the minimal
// ffmpeg-minimal build (~4 MB) from Lamprozx/ffmpeg-minimal. It returns nil if
// ffmpeg is available (or was just installed), otherwise an error.
func EnsureFFmpeg(neededFor string) error {
	if FFmpegAvailable() {
		return nil
	}
	fmt.Fprintf(os.Stderr, "ffmpeg not found in PATH, but %s requires it.\n", neededFor)

	url, installPath, ok := minimalFFmpegTarget()
	if !ok || !stdinIsTerminal() {
		fmt.Fprint(os.Stderr, FFmpegInstallHint())
		return errFFmpegMissing
	}

	fmt.Fprintf(os.Stderr, "OpenCall can install the minimal ffmpeg (~4 MB) from %s\n  -> %s\n",
		ffmpegMinimalRepo, installPath)
	if !confirm("Install now? [y/N] ") {
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, FFmpegInstallHint())
		return errFFmpegMissing
	}
	if err := downloadInstall(url, installPath); err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n%s", err, FFmpegInstallHint())
		return err
	}
	if FFmpegAvailable() {
		fmt.Fprintf(os.Stderr, "installed ffmpeg at %s\n", installPath)
	} else {
		fmt.Fprintf(os.Stderr, "installed ffmpeg at %s — add %q to your PATH and retry.\n",
			installPath, filepath.Dir(installPath))
	}
	return nil
}

// FFmpegInstallHint returns manual install instructions for the minimal build.
func FFmpegInstallHint() string {
	return fmt.Sprintf(`Install the minimal ffmpeg (~4 MB) instead of the full ~347 MB package:
  %s

  Termux (Android arm64):
    curl -L -o "$PREFIX/bin/ffmpeg" %s/ffmpeg-min-arm64
    chmod +x "$PREFIX/bin/ffmpeg"

  Linux (amd64):
    sudo curl -L -o /usr/local/bin/ffmpeg %s/ffmpeg-min-linux-amd64
    sudo chmod +x /usr/local/bin/ffmpeg
`, ffmpegMinimalRepo, ffmpegMinimalBase, ffmpegMinimalBase)
}

// minimalFFmpegTarget returns the prebuilt asset URL and install path for the
// current platform, or ok=false when no matching asset exists.
func minimalFFmpegTarget() (url, installPath string, ok bool) {
	switch {
	case isAndroid():
		prefix := os.Getenv("PREFIX")
		if prefix == "" {
			return "", "", false
		}
		return ffmpegMinimalBase + "/ffmpeg-min-arm64", filepath.Join(prefix, "bin", "ffmpeg"), true
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", false
		}
		return ffmpegMinimalBase + "/ffmpeg-min-linux-amd64", filepath.Join(home, ".local", "bin", "ffmpeg"), true
	default:
		return "", "", false
	}
}

func isAndroid() bool {
	return runtime.GOOS == "android" || os.Getenv("ANDROID_ROOT") != ""
}

func stdinIsTerminal() bool {
	return console.IsTerminal(int(os.Stdin.Fd()))
}

func confirm(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func downloadInstall(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}
	return verifyFFmpeg(dest)
}

func verifyFFmpeg(path string) error {
	out, err := exec.Command(path, "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s is not a working ffmpeg: %w: %s", path, err, out)
	}
	return nil
}
