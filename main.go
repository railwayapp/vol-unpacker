package main

import (
	"log/slog"
	"os"
	"path"
	"strings"
	"time"
)

func main() {
	volumeID := os.Getenv("RAILWAY_VOLUME_ID")
	volumeName := os.Getenv("RAILWAY_VOLUME_NAME")
	volumeMountPath := os.Getenv("RAILWAY_VOLUME_MOUNT_PATH")

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

	time.Sleep(time.Second * 10000)

}
