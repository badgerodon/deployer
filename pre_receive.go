package main

import (
	"fmt"
	"github.com/go-contrib/uuid"
	"github.com/moraes/config"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func PreReceive(dir, oldrev, newrev, ref string) error {
	// We only care about master
	if ref != "refs/heads/master" {
		return nil
	}

	temp := filepath.Join(os.TempDir(), uuid.NewV4().String())
	err := os.Mkdir(temp, 0777)
	if err != nil {
		return fmt.Errorf("- failed to create directory: %v", err)
	}
	//defer os.RemoveAll(temp)

	// Export to temp
	log.Println("exporting", dir, newrev, "to", temp)
	os.Chdir(dir)
	bs, err := exec.Command("bash", "-c", "git archive --format=tar "+newrev+" | tar -C "+temp+" -x ").CombinedOutput()
	if err != nil {
		return fmt.Errorf("- failed to export: %s", bs)
	}
	log.Println("-", string(bs))

	// Get config
	log.Println("reading config")
	os.Chdir(temp)
	cfg, err := config.ParseJsonFile("config.json")
	if err != nil {
		return fmt.Errorf("- failed to read config: %v", err)
	}
	apps, err := cfg.Map("")
	if err != nil {
		return fmt.Errorf("- failed to read applications: %v", err)
	}

	// Build
	for k, v := range apps {
		folder, err := cfg.String(k + ".folder")
		if err != nil {
			return fmt.Errorf("- expected folder in: %v, %v", v, err)
		}
		typ, err := cfg.String(k + ".type")
		if err != nil {
			return fmt.Errorf("- expected type in: %v, %v", v, err)
		}
		build, err := cfg.String(k + ".build")
		if err != nil {
			return fmt.Errorf("- expected build in: %v, %v", v, err)
		}

		switch build {
		case "go":
			err = BuildGo(typ, folder)
		default:
			err = fmt.Errorf("unknown build type %v", build)
		}

		if err != nil {
			return fmt.Errorf("error building %v: %v", k, err)
		}
	}

	// Clean
	log.Println("cleaning")
	filepath.Walk(temp, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(filepath.Base(path), ".") {
				os.RemoveAll(path)
				return filepath.SkipDir
			}
		} else {
			if strings.HasSuffix(path, ".go") {
				os.Remove(path)
			}
		}

		return nil
	})
	// Sync to endpoints
	// Disable app in load balancer
	// Swap to new version
	// Start the app
	// Enable app in load balancer
	// Cleanup old versions on the endpoints

	return fmt.Errorf("Not Implemented")
}
