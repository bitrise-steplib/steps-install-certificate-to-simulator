package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/fileutil"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/progress"
	"github.com/bitrise-io/steps-xcode-test/xcodeutil"
	"github.com/bitrise-tools/go-steputils/input"
)

// ConfigsModel ...
type ConfigsModel struct {
	CertificatePath    string
	SimulatorDevice    string
	SimulatorOsVersion string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		CertificatePath:    os.Getenv("certificate_path"),
		SimulatorDevice:    os.Getenv("simulator_device"),
		SimulatorOsVersion: os.Getenv("simulator_os_version"),
	}
}

func (configs ConfigsModel) print() {
	fmt.Println()
	log.Infof("Configs:")
	log.Printf("- CertificatePath: %s", configs.CertificatePath)
	log.Printf("- SimulatorDevice: %s", configs.SimulatorDevice)
	log.Printf("- SimulatorOsVersion: %s", configs.SimulatorOsVersion)
}

func (configs ConfigsModel) validate() error {
	// required
	if err := input.ValidateIfPathExists(configs.CertificatePath); err != nil {
		return fmt.Errorf("certificate_path - %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.SimulatorDevice); err != nil {
		return fmt.Errorf("simulator_device - %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.SimulatorOsVersion); err != nil {
		return fmt.Errorf("simulator_os_version - %s", err)
	}

	return nil
}

func devicesDir() string {
	userHome := pathutil.UserHomeDir()
	return filepath.Join(userHome, "/Library/Developer/CoreSimulator/Devices/")
}

func deviceDir(udid string) string {
	devicesDir := devicesDir()
	return filepath.Join(devicesDir, udid)
}

// DeviceTrustStorePath ...
func DeviceTrustStorePath(udid string) string {
	deviceDir := deviceDir(udid)
	return filepath.Join(deviceDir, "/data/Library/Keychains/TrustStore.sqlite3")
}

func main() {
	configs := createConfigsModelFromEnvs()
	configs.print()
	if err := configs.validate(); err != nil {
		log.Errorf("Issue with input: %s", err)
		os.Exit(1)
	}

	fmt.Println()
	log.Infof("Simulator infos:")

	simulator, err := xcodeutil.GetSimulator("iOS Simulator", configs.SimulatorDevice, configs.SimulatorOsVersion)
	if err != nil {
		log.Errorf("Failed to get simulator udid, error: %s", err)
		os.Exit(1)
	}

	log.Printf("- name: %s", simulator.Name)
	log.Printf("- udid: %s", simulator.SimID)
	log.Printf("- status: %s", simulator.Status)

	xcodebuildVersion, err := xcodeutil.GetXcodeVersion()
	if err != nil {
		log.Errorf("Failed to get the version of xcodebuild, error: %s", err)
		os.Exit(1)
	}

	log.Printf("- xcodebuild versions: %s", xcodebuildVersion.Version)

	if simulator.Status == "Shutdown" {
		log.Infof("Booting simulator (%s)...", simulator.SimID)

		if err := xcodeutil.BootSimulator(simulator, xcodebuildVersion); err != nil {
			log.Errorf("Failed to boot simulator, error: %s", err)
			os.Exit(1)
		}

		progress.NewDefaultWrapper("Waiting for simulator boot").WrapAction(func() {
			time.Sleep(30 * time.Second)
		})

		fmt.Println()
	}

	trustStorePth := DeviceTrustStorePath(simulator.SimID)

	log.Printf("- trust store path: %s", trustStorePth)

	tmpDir, err := pathutil.NormalizedOSTempDirPath("__cert-manager__")
	if err != nil {
		log.Errorf("Failed to get tmp dir, error: %s", err)
		os.Exit(1)
	}

	certificaterManagerScriptPth := filepath.Join(tmpDir, "iosCertTrustManager.py")
	if err := fileutil.WriteStringToFile(certificaterManagerScriptPth, TrustManagerScriptContent); err != nil {
		log.Errorf("Failed to write file, error: %s", err)
		os.Exit(1)
	}

	if err := os.Chmod(certificaterManagerScriptPth, 0770); err != nil {
		log.Errorf("Failed to set executable permission, error: %s", err)
		os.Exit(1)
	}

	cmd := command.NewWithStandardOuts(certificaterManagerScriptPth, "-a", configs.CertificatePath, "-t", trustStorePth)
	cmd.SetStdin(strings.NewReader("y"))

	fmt.Println()
	log.Donef("$ %s", cmd.PrintableCommandArgs())

	if err := cmd.Run(); err != nil {
		log.Errorf("Command failed, error: %s", err)
		os.Exit(1)
	}
}
