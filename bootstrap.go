package main

import (
	"code.google.com/p/go.crypto/ssh"
	"fmt"
	"github.com/badgerodon/sshutil"
	"log"

	//"github.com/kylelemons/go-gypsy/yaml"
)

type (
	bootstrapper struct {
		conn *ssh.ClientConn
	}
)

func (this *bootstrapper) run(args ...interface{}) error {
	session, err := this.conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	err = session.RequestPty("bash", 40, 80, ssh.TerminalModes{})
	if err != nil {
		return err
	}

	cmd := ""
	for i, arg := range args {
		if i > 0 {
			cmd += " "
		}
		cmd += fmt.Sprint(arg)
	}

	log.Println("run", cmd)
	bs, err := session.CombinedOutput(cmd)
	log.Println("- ", string(bs))
	return err
}

func (this *bootstrapper) bootstrap() error {
	return this.run("sudo", "mkdir", "-p", "/opt/deployer")
}

func Bootstrap(hostname string) error {
	conn, err := sshutil.Dial(hostname)
	if err != nil {
		return err
	}
	defer conn.Close()

	return (&bootstrapper{conn}).bootstrap()
}
