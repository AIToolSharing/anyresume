#!/bin/sh
# Downloads the anyresume release binary for this computer into bin/.
# herdr runs this script when it installs the plugin on Linux or macOS.
# When the download fails and Go is installed, the script builds the binary
# from the source.
set -eu

version="$1"
case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*)
	echo "anyresume: this script does not support $(uname -s)" >&2
	exit 1
	;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*)
	echo "anyresume: this script does not support the CPU $(uname -m)" >&2
	exit 1
	;;
esac

url="https://github.com/AIToolSharing/anyresume/releases/download/v${version}/anyresume_${os}_${arch}.tar.gz"
mkdir -p bin
if curl -fsSL "$url" | tar -xzf - -C bin anyresume; then
	exit 0
fi
if command -v go >/dev/null 2>&1; then
	echo "anyresume: the download failed; building from the source" >&2
	go build -o bin/anyresume .
	exit 0
fi
echo "anyresume: cannot download $url, and Go is not installed" >&2
exit 1
