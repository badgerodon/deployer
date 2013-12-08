package main

import (
	"code.google.com/p/go.crypto/ssh"
	"encoding/json"
	"github.com/badgerodon/sshutil"
	"github.com/go-contrib/uuid"
	"os"
	"path/filepath"
)

type (
	ProxyServer struct {
		Host string
		Port int
	}
	ProxyApplication struct {
		Name    string
		Domains []string
		Servers []*ProxyServer
	}
	ProxyConfig struct {
		Applications []*ProxyApplication
	}
)

func loadProxyConfig(conn *ssh.ClientConn) *ProxyConfig {
	str, _ := sshutil.Run(conn, "cat /etc/haproxy/config.json")
	var cfg ProxyConfig
	json.Unmarshal([]byte(str), &cfg)
	return &cfg
}

func saveProxyConfig(conn *ssh.ClientConn, cfg *ProxyConfig) error {
	tmp1 := filepath.Join(os.TempDir(), uuid.NewV4().String())
	defer os.Remove(tmp1)

	f, err := os.Create(tmp1)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(cfg)
	f.Close()
	if err != nil {
		return err
	}

	err = sshutil.SendFile(conn, tmp1, "/etc/haproxy/config.json")
	if err != nil {
		return err
	}

	tmp2 := filepath.Join(os.TempDir(), uuid.NewV4().String())
	defer os.Remove(tmp2)

	f, err = os.Create(tmp2)
	if err != nil {
		return err
	}
	err = HAPROXY.Execute(f, cfg)
	if err != nil {
		return err
	}

	err = sshutil.SendFile(conn, tmp2, "/etc/haproxy/haproxy.cfg")
	if err != nil {
		return err
	}

	_, err = sshutil.Run(conn, "systemctl reload haproxy")
	if err != nil {
		return err
	}

	return nil
}

func disableApplicationInProxy(conn *ssh.ClientConn, name string) error {
	cfg := loadProxyConfig(conn)
	for _, app := range cfg.Applications {
		if app.Name == name {
			app.Servers = []*ProxyServer{}
		}
	}
	return saveProxyConfig(conn, cfg)
}

func addApplicationToProxy(conn *ssh.ClientConn, app *ProxyApplication) error {
	cfg := loadProxyConfig(conn)
	nApps := []*ProxyApplication{}
	for _, a := range cfg.Applications {
		if a.Name != app.Name {
			nApps = append(nApps, a)
		}
	}
	nApps = append(nApps, app)
	cfg.Applications = nApps
	return saveProxyConfig(conn, cfg)
}
