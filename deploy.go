package main

import (
	"code.google.com/p/go.crypto/ssh"
	"fmt"
	"github.com/badgerodon/sshutil"
	"github.com/go-contrib/uuid"
	"github.com/moraes/config"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func copyAll(from, to string) error {
	return filepath.Walk(from, func(p string, fi os.FileInfo, err error) error {
		if err != nil || p == from {
			return nil
		}
		f := p[len(from):]
		if fi.IsDir() {
			err = os.Mkdir(filepath.Join(to, f), 0777)
		} else {
			ff, err := os.Open(p)
			if err != nil {
				return err
			}
			defer ff.Close()
			tf, err := os.Create(filepath.Join(to, f))
			if err != nil {
				return err
			}
			defer tf.Close()
			_, err = io.Copy(tf, ff)
		}

		return err
	})
}

func deployRestart(c *ssh.ClientConn, name string) error {
	tn := uuid.NewV4().String()
	tf := filepath.Join(os.TempDir(), tn)
	defer os.Remove(tf)

	ns, err := sshutil.Run(c, "netstat -lnt")
	if err != nil {
		return err
	}
	ports := map[int]bool{}
	for _, s := range strings.Fields(ns) {
		if strings.HasPrefix(s, ":::") {
			p, _ := strconv.Atoi(s[3:])
			ports[p] = true
		}
	}
	port := 9000
	for {
		if _, ok := ports[port]; !ok {
			break
		}
		port++
	}

	sshutil.Run(c, "sudo systemctl stop "+name+".service")
	sshutil.Run(c, "rm -rf /opt/"+name+"/current/*")
	sshutil.Run(c, "cp -R /opt/"+name+"/staging/* /opt/"+name+"/current/")

	f, err := os.Create(tf)
	if err != nil {
		return err
	}
	err = SYSTEMD.Execute(f, map[string]interface{}{
		"Name": name,
		"Port": port,
	})
	f.Close()
	if err != nil {
		return err
	}

	err = sshutil.SendFile(c, tf, "/tmp/"+tn)
	if err != nil {
		return err
	}
	sshutil.Run(c, `sudo mv -f /tmp/`+tn+` /etc/systemd/system/`+name+`.service`)
	sshutil.Run(c, `sudo systemctl enable `+name+`.service`)
	sshutil.Run(c, `sudo systemctl start `+name+`.service`)
	return nil
}

func prepareGo(root string) error {
	err := os.Chdir(root)
	if err != nil {
		return fmt.Errorf("unable to change directory to %v: %v", root, err)
	}

	_, err = os.Stat(filepath.Join(root, "Godeps"))
	if err != nil {
		err = exec.Command("godep", "save").Run()
	}
	if err != nil {
		return err
	}

	err = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || p == "." {
			return nil
		}

		if fi.IsDir() {
			if strings.HasPrefix(p, ".") {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func Deploy(root, env string) error {

	cfg, err := config.ParseJsonFile(filepath.Join(root, "config.json"))
	if err != nil {
		return err
	}

	apps, _ := cfg.Map("")

	for k, _ := range apps {
		app, _ := cfg.Get(k)

		folder, err := app.String("folder")
		if err != nil {
			return fmt.Errorf("Expected `folder` in app config")
		}
		build, err := app.String("build")
		if err != nil {
			return fmt.Errorf("Expected `build` in app config")
		}
		typ, err := app.String("type")
		if err != nil {
			return fmt.Errorf("Expected `type` in app config")
		}

		// create a temporary directory to store stuff
		temp := filepath.Join(os.TempDir(), uuid.NewV4().String())
		err = os.Mkdir(temp, 0777)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		defer os.RemoveAll(temp)

		// copy everything from the source into it
		err = copyAll(filepath.Join(root, folder), temp)
		if err != nil {
			return err
		}

		log.Println("[deploy]", "preparing", k)
		switch build {
		case "go":
			err = prepareGo(temp)
		}
		if err != nil {
			return err
		}

		envCfg, err := app.Get(env)
		if err != nil {
			return fmt.Errorf("Unknown environment: %v", env)
		}
		nodes, err := envCfg.List("nodes")
		if err != nil {
			return fmt.Errorf("Expected `nodes` in env config")
		}
		proxies, err := envCfg.List("proxies")
		if typ == "web" && err != nil {
			return fmt.Errorf("Expected `proxies` in env config")
		} else {
			proxies = []interface{}{}
		}

		conns := make(map[string]*ssh.ClientConn)
		for _, machine := range append(append([]interface{}{}, nodes...), proxies...) {
			h := fmt.Sprint(machine)
			_, ok := conns[h]
			if !ok {
				c, err := sshutil.Dial(h)
				if err != nil {
					return err
				}
				defer c.Close()
				conns[h] = c
			}
		}

		log.Println("[deploy]", "staging", k)
		for _, node := range nodes {
			c := conns[fmt.Sprint(node)]
			str, err := sshutil.Run(c, `sudo sh -c "`+
				`mkdir -p /opt/`+k+`/staging && chmod 777 /opt/`+k+`/staging && `+
				`mkdir -p /opt/`+k+`/current && chmod 777 /opt/`+k+`/current `+
				`"`)
			if err != nil {
				return fmt.Errorf("[deploy] failed to setup folders on %v: %v", node, str)
			}

			err = sshutil.SyncFolder(c, temp, "/opt/"+k+"/staging")
			if err != nil {
				return err
			}
		}

		log.Println("[deploy]", "building", k)
		switch build {
		case "go":
			for _, node := range nodes {
				c := conns[fmt.Sprint(node)]
				str, err := sshutil.Run(c, `cd /opt/`+k+`/staging && godep go build -v -o `+k)
				if err != nil {
					return fmt.Errorf("[deploy] failed to build on %v: %v", node, str)
				}
			}
		}

		switch typ {
		case "web":
			// remove from proxies
		}

		log.Println("[deploy]", "restarting", k)
		for _, node := range nodes {
			c := conns[fmt.Sprint(node)]
			err = deployRestart(c, k)
			if err != nil {
				return err
			}
		}
		// stop processes
		// copy code
		// start processes

		switch typ {
		case "web":
			// add to proxies
		}

		log.Println(build, typ, folder)
	}

	return nil
}
