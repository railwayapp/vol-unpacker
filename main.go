package main

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/moby/go-archive"
	"github.com/moby/go-archive/compression"
)

func main() {
	volumeID := os.Getenv("RAILWAY_VOLUME_ID")
	volumeName := os.Getenv("RAILWAY_VOLUME_NAME")
	volumeMountPath := os.Getenv("RAILWAY_VOLUME_MOUNT_PATH")

	downloadLink := os.Getenv("TARBALL_URL")
	if downloadLink == "" {
		slog.Error("TARBALL_URL not set, aborting...")
		os.Exit(1)
	}

	if volumeID == "" {
		slog.Error("Volume ID not set, do you have a volume attached? Aborting...")
		os.Exit(1)
	}

	if volumeName == "" {
		slog.Error("Volume ID not set, do you have a volume attached? Aborting...")
		os.Exit(1)
	}

	if volumeMountPath == "" {
		slog.Error("Volume ID not set, do you have a volume attached? Aborting...")
		os.Exit(1)
	}

	dirEnts, err := os.ReadDir(volumeMountPath)
	if err != nil {
		slog.Error("failed ot list volume directory", "path", volumeMountPath, "error", err)
		os.Exit(2)
	}

	dirEntsCount := 0
	lostFoundExists := false
	for _, dirEnt := range dirEnts {
		if path.Base(dirEnt.Name()) == "lost+found" && dirEnt.IsDir() {
			lostFoundExists = true
			continue
		}
		slog.Info("found exiting file in volume", "name", dirEnt.Name())
		dirEntsCount++
	}

	if !lostFoundExists {
		slog.Error("lost+found directory not found in volume, is this a valid ext4 filesystem? aborting...")
		os.Exit(3)
	}
	ignoreExistingFiles := strings.ToLower(os.Getenv("IGNORE_EXISTING_FILES")) == "yes"

	if dirEntsCount > 0 && !ignoreExistingFiles {
		slog.Error("found exiting files in volume, will only work with an empty volume aborting...")
		slog.Error("to force unpack, set IGNORE_EXISTING_FILES=yes as an environment variable")
		os.Exit(4)
	}

	slog.Info("ready to unpack data to volume", "volume_id", volumeID, "volume_name", volumeName, "volume_mount_path", volumeMountPath)

	req, err := http.Get(downloadLink)
	if err != nil {
		slog.Error("failed to download tarball", "url", downloadLink, "error", err)
		os.Exit(5)
	}

	body := req.Body
	decompressStream, err := compression.DecompressStream(body)
	if err != nil {
		slog.Error("failed to decompress tarball", "url", downloadLink, "error", err)
		os.Exit(6)
	}

	mr := &meteredReader{
		counter: &atomic.Int64{},
		source:  decompressStream,
	}

	done := &atomic.Bool{}

	go func() {
		for done.Load() == false {
			time.Sleep(time.Second * 5)
			slog.Info("unpacking to disk", "bytes", mr.counter.Load())
		}
	}()

	if err := archive.Unpack(mr, volumeMountPath, &archive.TarOptions{}); err != nil {
		slog.Error("failed to unpack tarball", "url", downloadLink, "error", err)
		os.Exit(7)
	}

	done.Store(true)
	slog.Info("successfully unpacked volume data", "volume_id", volumeID, "volume_name", volumeName, "volume_mount_path", volumeMountPath, "bytes_processed", mr.counter.Load())
	os.Exit(0)
}

type meteredReader struct {
	counter *atomic.Int64
	source  io.Reader
}

func (mr *meteredReader) Read(p []byte) (n int, err error) {
	n, err = mr.source.Read(p)
	mr.counter.Add(int64(n))
	return n, err
}
