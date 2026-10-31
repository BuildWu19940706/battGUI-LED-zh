package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// This is a platform profile, not a machine identity. Do not retain or log
// serial numbers / UUIDs returned by system_profiler.
type firmwareLEDProfile struct {
	Model     string
	Firmware  string
	OSVersion string
}

// Initialized once before the daemon starts any worker or request handler.
// A missing profile is deliberately not a firmware-LED capability.
var currentFirmwareLEDProfile firmwareLEDProfile

func supportsFirmwareLED(profile firmwareLEDProfile) bool {
	// 2026-11-01: user confirmed root orange(4), green(3), system(0) writes,
	// matching readback and the physical green LED on this exact combination.
	return profile.Model == "Mac16,12" && profile.Firmware == "mBoot-20457.1.29" && profile.OSVersion == "15.8.1"
}

func parseFirmwareLEDProfile(hardware []byte, osVersion string) (firmwareLEDProfile, error) {
	var report struct {
		Hardware []struct {
			Model    string `json:"machine_model"`
			Firmware string `json:"boot_rom_version"`
		} `json:"SPHardwareDataType"`
	}
	if err := json.Unmarshal(hardware, &report); err != nil {
		return firmwareLEDProfile{}, fmt.Errorf("parse hardware profile: %w", err)
	}
	if len(report.Hardware) != 1 {
		return firmwareLEDProfile{}, errors.New("hardware profile has no unique hardware entry")
	}
	profile := firmwareLEDProfile{strings.TrimSpace(report.Hardware[0].Model), strings.TrimSpace(report.Hardware[0].Firmware), strings.TrimSpace(osVersion)}
	if profile.Model == "" || profile.Firmware == "" || profile.OSVersion == "" {
		return firmwareLEDProfile{}, errors.New("hardware profile is incomplete")
	}
	return profile, nil
}

func readFirmwareLEDProfile() (firmwareLEDProfile, error) {
	// A slow or unavailable profiler must never prevent charge control startup.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hardwareCommand := exec.CommandContext(ctx, "/usr/sbin/system_profiler", "SPHardwareDataType", "-json")
	hardwareCommand.WaitDelay = time.Second
	hardware, err := hardwareCommand.Output()
	if err != nil {
		return firmwareLEDProfile{}, fmt.Errorf("read hardware profile: %w", err)
	}
	versionCommand := exec.CommandContext(ctx, "/usr/bin/sw_vers", "-productVersion")
	versionCommand.WaitDelay = time.Second
	version, err := versionCommand.Output()
	if err != nil {
		return firmwareLEDProfile{}, fmt.Errorf("read OS version: %w", err)
	}
	return parseFirmwareLEDProfile(hardware, string(version))
}
