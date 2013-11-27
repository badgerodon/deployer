package main

import (
	"fmt"
	"github.com/badgerodon/proxy"
	"github.com/go-contrib/uuid"
	"github.com/moraes/config"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func DisableAppInProxy(cfg *config.Config) error {
	pcfg, err := proxy.GetConfig("/opt/proxy/config.json")
	if err != nil {
		return fmt.Errorf("error reading proxy config: %v", err)
	}
	host, err := cfg.String("host")
	if err != nil {
		return fmt.Errorf("error reading host: %v", err)
	}
	delete(pcfg.Routes, host)
	err = pcfg.Save("/opt/proxy/config.json")
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}
	return nil
}
func EnableAppInProxy(cfg *config.Config) error {
	pcfg, err := proxy.GetConfig("/opt/proxy/config.json")
	if err != nil {
		return fmt.Errorf("error reading proxy config: %v", err)
	}
	host, err := cfg.String("host")
	if err != nil {
		return fmt.Errorf("error reading host: %v", err)
	}
	port, err := cfg.Int("port")
	if err != nil {
		return fmt.Errorf("error reading port: %v", err)
	}
	pcfg.Routes[host] = proxy.Entry{
		Endpoints: []string{fmt.Sprint("127.0.0.1:", port)},
	}
	err = pcfg.Save("/opt/proxy/config.json")
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}
	return nil
}

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
	defer os.RemoveAll(temp)

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
		app, _ := cfg.Get(k)

		_, err = app.String("folder")
		if err != nil {
			return fmt.Errorf("- expected folder in: %v, %v", v, err)
		}
		_, err = app.String("type")
		if err != nil {
			return fmt.Errorf("- expected type in: %v, %v", v, err)
		}
		build, err := app.String("build")
		if err != nil {
			return fmt.Errorf("- expected build in: %v, %v", v, err)
		}

		switch build {
		case "go":
			err = BuildGo(temp, k, app)
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
			if strings.HasPrefix(filepath.Base(path), ".") || filepath.Base(path) == "Godeps" {
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

	for k, _ := range apps {
		app, _ := cfg.Get(k)
		typ, _ := app.String("type")
		folder, _ := app.String("folder")

		// Sync to endpoints
		log.Println("syncing", k)
		os.Mkdir("/opt/"+k, 0777)
		os.Mkdir("/opt/"+k+"/staging", 0777)
		bs, err := exec.Command("rsync",
			"--recursive",
			"--links",
			"--perms",
			"--times",
			"--devices",
			"--specials",
			"--hard-links",
			"--acls",
			"--delete",
			"--xattrs",
			"--numeric-ids",
			filepath.Join(temp, folder)+"/", // from
			"/opt/"+k+"/staging/",           // to
		).CombinedOutput()
		if err != nil {
			return fmt.Errorf("error syncing folder: %s", bs)
		}

		// Disable app in load balancer
		if typ == "web" {
			err = DisableAppInProxy(app)
			if err != nil {
				return fmt.Errorf("error disabling app in proxy: %v", err)
			}
		}

		// Stop the app
		log.Println("stopping")
		exec.Command("/etc/init.d/"+k, "stop").Run()

		// Swap to new version
		log.Println("swapping")
		os.Mkdir("/opt/"+k+"/current", 0777)
		bs, err = exec.Command("rsync",
			"--recursive",
			"--links",
			"--perms",
			"--times",
			"--devices",
			"--specials",
			"--hard-links",
			"--acls",
			"--delete",
			"--xattrs",
			"--numeric-ids",
			"/opt/"+k+"/staging/", // from
			"/opt/"+k+"/current/", // to
		).CombinedOutput()
		if err != nil {
			return fmt.Errorf("error syncing folder: %s", bs)
		}
		// Start the app
		log.Println("starting")
		exec.Command("/etc/init.d/"+k, "start").Run()

		// Enable app in load balancer
		if typ == "web" {
			err = EnableAppInProxy(app)
			if err != nil {
				return fmt.Errorf("error enabling app in proxy: %v", err)
			}
		}
	}

	return fmt.Errorf("Not Implemented")
}
