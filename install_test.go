package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScript_ResolvesLatestRelease(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	tmp := t.TempDir()
	fakeBin := filepath.Join(tmp, "fake-bin")
	installDir := filepath.Join(tmp, "install-bin")
	curlLog := filepath.Join(tmp, "curl.log")

	if err := os.MkdirAll(fakeBin, 0755); err != nil {
		t.Fatalf("MkdirAll(fakeBin): %v", err)
	}

	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CURL_LOG"
case "$*" in
  *"%{url_effective}"*)
    printf '%s' "https://github.com/broothie/veer/releases/tag/v9.8.7"
    exit 0
    ;;
esac
outfile=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    outfile="$2"
    shift 2
    continue
  fi
  shift
done
: > "$outfile"
`)
	writeExecutable(t, filepath.Join(fakeBin, "tar"), `#!/bin/sh
set -eu
dest=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-C" ]; then
    dest="$2"
    shift 2
    continue
  fi
  shift
done
printf '#!/bin/sh\n' > "$dest/veer"
chmod +x "$dest/veer"
`)
	writeExecutable(t, filepath.Join(fakeBin, "install"), `#!/bin/sh
set -eu
cp "$1" "$2"
`)

	cmd := exec.Command("sh", "scripts/install.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
		"HOME="+tmp,
		"BIN_DIR="+installDir,
		"CURL_LOG="+curlLog,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(installDir, "veer")); err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}

	curlCalls, err := os.ReadFile(curlLog)
	if err != nil {
		t.Fatalf("ReadFile(curlLog): %v", err)
	}
	if !strings.Contains(string(curlCalls), "releases/latest/download/veer_9.8.7_") {
		t.Fatalf("curl log %q does not contain resolved latest asset URL", string(curlCalls))
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
