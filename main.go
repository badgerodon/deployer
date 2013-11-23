package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"
	"time"
)

var (
	upstartScript = template.Must(template.New("upstart").Parse(`
description "{{.name}}"

start on runlevel [2345]
stop on runlevel [!2345]

pre-start script
    mkdir -p /var/opt/{{.name}}
    mkdir -p /var/opt/{{.name}}/log
    mkdir -p /var/opt/{{.name}}/run
    chown -R {{.user}}:{{.user}} /var/opt/{{.name}}
end script

respawn

exec start-stop-daemon --start \
  --chuid {{.user}} \
  --chdir /server/deploy/{{.name}} \
  --make-pidfile \
  --pidfile /var/opt/{{.name}}/run/{{.name}}-upstart.pid \
  --exec /server/deploy/{{.name}}/{{.name}} \
  >> /var/opt/{{.name}}/log/{{.name}}-upstart.log 2>&1
`))
	postDeployScript = template.Must(template.New("postdeploy").Parse(`
set -x verbose

# build
cd /server/staging/{{.name}}
go build -o {{.name}} ./src

# stop existing service
sudo systemctl stop {{.name}}.service
sudo stop {{.name}}-upstart
# copy staging to deploy
rm -rf /server/deploy/{{.name}}
mkdir --parents /server/deploy/{{.name}}/
rsync --perms --times --recursive --delete /server/staging/{{.name}}/ /server/deploy/{{.name}}
# copy new init script
sudo cp -f /server/deploy/{{.name}}/{{.name}}.service /etc/systemd/system/{{.name}}.service
sudo cp -f /server/deploy/{{.name}}/{{.name}}-upstart.conf /etc/init/{{.name}}-upstart.conf
rm -f /server/deploy/{{.name}}/postdeploy.sh
rm -f /server/deploy/{{.name}}/{{.name}}.service
rm -f /server/deploy/{{.name}}/{{.name}}-upstart.conf
# start new service
sudo systemctl enable {{.name}}.service
sudo systemctl start {{.name}}.service
sudo start {{.name}}-upstart
  `))
	systemdScript = template.Must(template.New("systemd").Parse(`
[Unit]
Description={{.name}}
After=syslog.target
After=network.target

[Service]
Type=simple
ExecStart=/server/deploy/{{.name}}/{{.name}}
User=server
Group=server
Restart=always
WorkingDirectory=/server/deploy/{{.name}}/
Environment=PORT={{.port}}

[Install]
WantedBy=multi-user.target
  `))
)

type (
	Config struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		Name string `json:"name"`
		User string `json:"user"`
	}
)

func (this Config) run(command string, args ...string) error {
	log.Println("|", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (this Config) remote(command string) error {
	return this.run("ssh",
		this.User+"@"+this.Host,
		command,
	)
}

func deploy() error {
	var err error

	cfgFile, err := os.Open("app.json")
	if err != nil {
		return err
	}
	defer cfgFile.Close()
	var cfg Config
	err = json.NewDecoder(cfgFile).Decode(&cfg)
	if err != nil {
		return err
	}
	folder := path.Join(os.TempDir(), cfg.Name) + "/"

	log.Println("Archiving")
	cfg.run("rm", "-rf", folder)
	err = cfg.run("mkdir", folder)
	if err != nil {
		return err
	}
	defer cfg.run("rm", "-rf", folder)

	err = cfg.run("git", "checkout-index", "-a", "-f", "--prefix", folder)
	if err != nil {
		return err
	}
	err = os.Chdir(folder)
	if err != nil {
		return err
	}

	log.Println("Building")
	err = cfg.run("go", "build", "-o", cfg.Name, "./src")
	if err != nil {
		return err
	}

	log.Println("Writing Scripts")
	settings := map[string]string{
		"user":    cfg.User,
		"name":    cfg.Name,
		"port":    fmt.Sprint(cfg.Port),
		"version": fmt.Sprint(time.Now().Unix()),
	}

	file, err := os.OpenFile(
		path.Join(folder, "postdeploy.sh"),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		0777,
	)
	if err != nil {
		return err
	}
	err = postDeployScript.Execute(file, settings)
	file.Close()
	if err != nil {
		return err
	}
	file, err = os.Create(path.Join(folder, cfg.Name+".service"))
	if err != nil {
		return err
	}
	err = systemdScript.Execute(file, settings)
	file.Close()
	if err != nil {
		return err
	}
	file, err = os.Create(path.Join(folder, cfg.Name+"-upstart.conf"))
	if err != nil {
		return err
	}
	err = upstartScript.Execute(file, settings)
	file.Close()

	log.Println("Syncing")
	err = cfg.run("rsync",
		"--compress",
		"--perms",
		"--times",
		"--recursive",
		"--delete",
		//"--exclude", "src/",
		folder,
		cfg.User+"@"+cfg.Host+":/server/staging/"+cfg.Name,
	)
	if err != nil {
		return err
	}

	log.Println("Postdeploy")
	err = cfg.remote("/server/staging/" + cfg.Name + "/postdeploy.sh")
	if err != nil {
		return err
	}

	return nil
}

func main() {
	log.SetFlags(log.Lshortfile)

	args := os.Args
	if len(args) <= 1 {
		log.Fatalln("Expected `mode`")
	}
	mode := args[1]

	var err error

	switch mode {
	case "bootstrap":
		if len(args) <= 2 {
			log.Fatalln("Expected `hostname`")
		}
		hostname := args[2]
		err = Bootstrap(hostname)
	case "pre-receive":
		if len(args) <= 5 {
			log.Fatalln("Expected `dir`, `oldrev`, `newrev`, `ref`")
		}
		dir := args[2]
		oldrev := args[3]
		newrev := args[4]
		ref := args[5]
		err = PreReceive(dir, oldrev, newrev, ref)
	default:
		err = fmt.Errorf("Unknown mode `%v`", mode)
	}

	if err != nil {
		log.Fatalln(err)
	}
}
