package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
		if filepath.Base(dirEnt.Name()) == "lost+found" && dirEnt.IsDir() {
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

	err = os.MkdirAll("/tmp/untar", 0755)
	if err != nil {
		slog.Error("failed to create /tmp/untar directory", "error", err)
		os.Exit(7)
	}

	if err := archive.Unpack(mr, "/tmp/untar", &archive.TarOptions{}); err != nil {
		slog.Error("failed to unpack tarball", "url", downloadLink, "error", err)
		os.Exit(7)
	}

	// the extracted tarball will produce
	// {deployId}/{externalId}/[actual content]
	// we want to keep only the actual content and move it to the root of the volume

	deployInstanceId, err := extractUUIDFromTarballURL(downloadLink)
	if err != nil {
		slog.Error("failed to extract UUID from tarball URL", "url", downloadLink, "error", err)
		os.Exit(8)
	}

	// get externalID
	subDirs, err := os.ReadDir(fmt.Sprintf("/tmp/untar/%s", deployInstanceId))
	if err != nil {
		slog.Error("failed to read directory", "path", fmt.Sprintf("/tmp/untar/%s", deployInstanceId), "error", err)
		os.Exit(9)
	}
	var externalId string
	for _, dir := range subDirs {
		if !dir.IsDir() {
			continue
		}
		if strings.HasPrefix(dir.Name(), "vol_") {
			externalId = dir.Name()
			break
		}
	}
	if externalId == "" {
		slog.Error("unexpected directory contents", "path", fmt.Sprintf("/tmp/untar/%s", deployInstanceId), "contents", subDirs)
		os.Exit(10)
	}

	// Instead of using Go's file operations, use bash with -c to properly handle glob patterns
	sourceGlob := fmt.Sprintf("/tmp/untar/%s/%s/*", deployInstanceId, externalId)
	destination := fmt.Sprintf("%s/", volumeMountPath)
	
	// Use bash -c to ensure glob expansion works
	cmd := exec.Command("bash", "-c", fmt.Sprintf("mv %s %s", sourceGlob, destination))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		slog.Error("failed to move directory contents", "source", sourceGlob, "destination", destination, "error", err)
		os.Exit(11)
	}

	done.Store(true)
	slog.Info("successfully unpacked volume data", "volume_id", volumeID, "volume_name", volumeName, "volume_mount_path", volumeMountPath, "bytes_processed", mr.counter.Load())
	os.Exit(0)
}

func extractUUIDFromTarballURL(tarballURL string) (string, error) {
	parsedURL, err := url.Parse(tarballURL)
	if err != nil {
		return "", err
	}

	filename := filepath.Base(parsedURL.Path)
	uuid := strings.TrimSuffix(filename, ".tgz")
	return uuid, nil
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
