package main

import (
	"text/template"
)

var (
	SYSTEMD = template.Must(template.New("systemd").Parse(`
[Unit]
Description={{.Name}}
After=syslog.target
After=network.target

[Service]
Type=simple
ExecStart=/opt/{{.Name}}/current/{{.Name}}
User=fedora
Group=fedora
Restart=always
WorkingDirectory=/opt/{{.Name}}/current
Environment=PORT={{.Port}}

[Install]
WantedBy=multi-user.target
  `))
)
