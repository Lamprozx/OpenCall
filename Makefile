# OpenCall — build targets.
#
#   make           build amd64 + arm64 (alias for `all`)
#   make amd64     static linux/amd64 binary      -> OpenCall-linux-amd64
#   make arm64     android/arm64 binary (Termux)  -> OpenCall-android-arm64
#   make build     native build for this host     -> OpenCall
#   make test      go test ./...
#   make vet       go vet ./...
#   make clean     remove built binaries
#
# The arm64 target cross-compiles for Android using the NDK so DNS resolves
# natively in Termux (no proot/resolv.conf tricks). Set ANDROID_NDK_HOME to an
# unpacked NDK (r26d recommended):
#   https://developer.android.com/ndk/downloads

PKG     := ./cmd/opencall
LDFLAGS := -s -w

ANDROID_NDK_HOME ?= /opt/android-ndk-r26d
NDK_TOOLCHAIN   := $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64
NDK_CC          := $(NDK_TOOLCHAIN)/bin/aarch64-linux-android24-clang
NDK_CXX         := $(NDK_TOOLCHAIN)/bin/aarch64-linux-android24-clang++

.PHONY: all amd64 arm64 build test vet clean

all: amd64 arm64

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o OpenCall $(PKG)

amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o OpenCall-linux-amd64 $(PKG)

arm64:
	@test -x "$(NDK_CC)" || { \
		echo "Android NDK not found at $(ANDROID_NDK_HOME)."; \
		echo "Install NDK r26d and re-run, or set ANDROID_NDK_HOME:"; \
		echo "  make arm64 ANDROID_NDK_HOME=/path/to/android-ndk-r26d"; \
		exit 1; \
	}
	CC="$(NDK_CC)" CXX="$(NDK_CXX)" CGO_ENABLED=1 GOOS=android GOARCH=arm64 \
		go build -ldflags="$(LDFLAGS)" -o OpenCall-android-arm64 $(PKG)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f OpenCall OpenCall-linux-amd64 OpenCall-android-arm64
