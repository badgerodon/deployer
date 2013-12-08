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
	"regexp"
	"strconv"
	"strings"
)

var (
	IP_PATTERN = regexp.MustCompile(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`)
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

func deployRestart(c *ssh.ClientConn, name, env string) (int, error) {
	tn := uuid.NewV4().String()
	tf := filepath.Join(os.TempDir(), tn)
	defer os.Remove(tf)

	ns, err := sshutil.Run(c, "netstat -lnt")
	if err != nil {
		return 0, err
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

	sshutil.Run(c, "systemctl stop "+name+".service")
	sshutil.Run(c, "rm -rf /opt/"+name+"/"+env+"/*")
	sshutil.Run(c, "cp -R /opt/"+name+"/_staging/* /opt/"+name+"/"+env+"/")
	cleanRemote(c, "/opt/"+name+"/"+env)

	f, err := os.Create(tf)
	if err != nil {
		return 0, err
	}
	err = SYSTEMD.Execute(f, map[string]interface{}{
		"Name": name,
		"Port": port,
		"Env":  env,
	})
	f.Close()
	if err != nil {
		return 0, err
	}

	err = sshutil.SendFile(c, tf, "/etc/systemd/system/"+name+".service")
	if err != nil {
		return 0, err
	}
	sshutil.Run(c, `systemctl enable `+name+`.service`)
	sshutil.Run(c, `systemctl start `+name+`.service`)
	return port, nil
}

func cleanLocal(root string) error {
	return filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() && strings.HasPrefix(filepath.Base(p), ".") {
			os.RemoveAll(p)
			return filepath.SkipDir
		}
		return nil
	})
}

func cleanRemote(conn *ssh.ClientConn, root string) error {
	_, err := sshutil.Run(conn, `find `+root+` -name '*.go' -print0 | xargs -0 rm -rf`)
	return err
}

func prepareGo(root string) error {
	err := os.Chdir(root)
	if err != nil {
		return fmt.Errorf("unable to change directory to %v: %v", root, err)
	}
	os.MkdirAll(filepath.Join(root, "vendor", "src"), 0700)

	deps, err := exec.Command("go", "list", "-f", "{{range .Deps}}{{.}} {{end}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get dependencies: %s", deps)
	}
	args := append([]string{"list", "-f", "{{.Standard}} {{.Dir}} {{.ImportPath}}"}, strings.Fields(string(deps))...)
	deps, err = exec.Command("go", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get dependencies: %s", deps)
	}
	for _, dep := range strings.Split(string(deps), "\n") {
		fs := strings.Fields(dep)
		if len(fs) > 2 && fs[0] == "false" {
			src := fs[1]
			importPath := fs[2]
			dst := filepath.Join(root, "vendor", "src", importPath)
			os.MkdirAll(dst, 0700)
			err = copyAll(src, dst)
			if err != nil {
				return fmt.Errorf("failed to copy dependency: %v", err)
			}
		}
	}

	return nil
}

func getStringList(cfg *config.Config, name string) []string {
	arr, err := cfg.List(name)
	if err != nil {
		return []string{}
	}
	items := []string{}
	for _, item := range arr {
		items = append(items, fmt.Sprint(item))
	}
	return items
}

func getIp(conn *ssh.ClientConn, hostname string) string {
	if IP_PATTERN.MatchString(hostname) {
		return hostname
	}
	str, err := sshutil.Run(conn, "ip addr show")
	if err != nil {
		return hostname
	}
	for _, ln := range strings.Split(str, "\n") {
		fs := strings.Fields(ln)
		if len(fs) > 1 && fs[0] == "inet" {
			ip := fs[1]
			if strings.Contains(ip, "/") {
				ip = ip[0:strings.Index(ip, "/")]
			}
			if ip != "127.0.0.1" {
				return ip
			}
		}
	}
	return hostname
}

func Deploy(root, env string) error {
	cfg, err := config.ParseJsonFile(filepath.Join(root, "deploy.json"))
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
		err = copyAll(root, temp)
		if err != nil {
			return err
		}

		log.Println("[deploy]", "preparing", k)
		switch build {
		case "go":
			err = prepareGo(filepath.Join(temp, folder))
		}
		if err != nil {
			return err
		}

		err = cleanLocal(temp)
		if err != nil {
			return err
		}

		envCfg, err := app.Get(env)
		if err != nil {
			return fmt.Errorf("Unknown environment: %v", env)
		}
		servers := getStringList(envCfg, "servers")
		proxies := getStringList(envCfg, "proxies")
		domains := getStringList(envCfg, "domains")

		conns := make(map[string]*ssh.ClientConn)
		for _, machine := range append(append([]string{}, servers...), proxies...) {
			_, ok := conns[machine]
			if !ok {
				c, err := sshutil.Dial(machine)
				if err != nil {
					return err
				}
				defer c.Close()
				conns[machine] = c
			}
		}

		log.Println("[deploy]", "staging", k)
		serverIps := map[string]string{}
		for _, server := range servers {
			c := conns[server]
			str, err := sshutil.Run(c,
				`mkdir -p /opt/`+k+`/_staging && chmod 777 /opt/`+k+`/_staging && `+
					`mkdir -p /opt/`+k+`/`+env+` && chmod 777 /opt/`+k+`/`+env+` `)
			if err != nil {
				return fmt.Errorf("[deploy] failed to setup folders on %v: %v", server, str)
			}

			err = sshutil.SyncFolder(c, temp, "/opt/"+k+"/_staging")
			if err != nil {
				return err
			}

			serverIps[server] = getIp(c, server)
		}

		log.Println("[deploy]", "building", k)
		switch build {
		case "go":
			for _, server := range servers {
				c := conns[server]
				str, err := sshutil.Run(c, `cd /opt/`+k+`/_staging/`+folder+` && GOPATH=/opt/`+k+`/_staging/`+folder+`/vendor go build -v -o /opt/`+k+`/_staging/`+k)
				if err != nil {
					return fmt.Errorf("[deploy] failed to build on %v: %v", server, str)
				}
			}
		}

		switch typ {
		case "web":
			for _, proxy := range proxies {
				c := conns[proxy]
				err = disableApplicationInProxy(c, k)
				if err != nil {
					return fmt.Errorf("[deploy] failed to disable application in proxy on %v: %v", proxy, err)
				}
			}
		}

		log.Println("[deploy]", "restarting", k)
		ports := make(map[string]int)
		for _, server := range servers {
			c := conns[server]
			port, err := deployRestart(c, k, env)
			if err != nil {
				return err
			}
			ports[server] = port
		}

		switch typ {
		case "web":
			proxyServers := []*ProxyServer{}
			for _, server := range servers {
				proxyServers = append(proxyServers, &ProxyServer{
					Host: serverIps[server],
					Port: ports[server],
				})
			}
			app := &ProxyApplication{
				Name:    k,
				Domains: domains,
				Servers: proxyServers,
			}
			// add to proxies
			for _, proxy := range proxies {
				c := conns[proxy]
				err = addApplicationToProxy(c, app)
				if err != nil {
					return fmt.Errorf("[deploy] failed to add application to proxy on %v: %v", proxy, err)
				}
			}
		}
	}

	return nil
}
