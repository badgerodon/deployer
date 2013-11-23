package main

import (
	"github.com/go-contrib/uuid"
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
	defer os.RemoveAll(temp)

	// Export to temp
	log.Println("[build]", "exporting", dir, newrev, "to", temp)
	os.Chdir(dir)
	bs, err := exec.Command("/bin/bash", "git archive --format tar | tar -C "+temp+" -x ").CombinedOutput()
	if err != nil {
		return err
	}
	log.Println("[build]", "-", string(bs))

	// Build
	log.Println("[build]", "building")
	os.Chdir(temp)
	bs, err = exec.Command("go", "build", "-v")
	if err != nil {
		return err
	}
	log.Println("[build]", "-", string(bs))

	// Clean
	log.Println("[build]", "cleaning")
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
}
