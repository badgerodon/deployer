package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func BuildGo(typ, folder string) error {
	log.Println("building as go:", typ, folder)
	err := os.Chdir(folder)
	if err != nil {
		return fmt.Errorf("unable to change directory to %v: %v", folder, err)
	}

	var cmd *exec.Cmd

	fi, err := os.Stat(filepath.Join(folder, "Godeps"))
	if err == nil && fi.IsDir() {
		cmd = exec.Command("godep", "go", "build", "-v")
	} else {
		cmd = exec.Command("go", "build", "-v")
	}

	bs, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build: %v, %v", string(bs), err)
	}

	return nil
}
