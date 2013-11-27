package main

import (
	"fmt"
	"github.com/moraes/config"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func BuildGo(root, name string, cfg *config.Config) error {
	folder, err := cfg.String("folder")
	if err != nil {
		return fmt.Errorf("expected folder in config")
	}
	folder = filepath.Join(root, folder)

	log.Println("building as go:", name, folder)
	err = os.Chdir(folder)
	if err != nil {
		return fmt.Errorf("unable to change directory to %v: %v", folder, err)
	}

	var cmd *exec.Cmd

	fi, err := os.Stat(filepath.Join(folder, "Godeps"))
	if err == nil && fi.IsDir() {
		cmd = exec.Command("godep", "go", "build", "-v", "-o", name)
	} else {
		cmd = exec.Command("go", "build", "-v", "-o", name)
	}

	bs, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build: %v, %v", string(bs), err)
	} else {
		log.Println(string(bs))
	}

	return nil
}
