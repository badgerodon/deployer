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
ExecStart=/opt/{{.Name}}/{{.Env}}/{{.Name}}
Restart=always
WorkingDirectory=/opt/{{.Name}}/{{.Env}}
Environment=PORT={{.Port}} ENV={{.Env}}

[Install]
WantedBy=multi-user.target
  `))

	HAPROXY = template.Must(template.New("haproxy").Parse(`
global
	maxconn 4096
	log /dev/log local0

defaults
	log global
	mode http
	option tcplog
	retries 4
	option redispatch
	maxconn 32000
	contimeout 5000
	clitimeout 30000
	srvtimeout 30000
	timeout client 30000

frontend frontend
	bind 0.0.0.0:80
	#bind 0.0.0.0:443 ssl crt /etc/haproxy/certs.d
	option httplog
	option http-pretend-keepalive
	option forwardfor
	option http-server-close
{{range .Applications}}
	use_backend {{.Name}} if { {{range .Domains}} hdr(host) -i {{.}} {{end}} }
{{end}}

{{range .Applications}}

backend {{.Name}}
	balance roundrobin
	reqadd X-Forwarded-Proto:\ https if { ssl_fc }
	option forwardfor
	option abortonclose
	#option httpchk GET /
{{range .Servers}}
	server {{.Host}}-{{.Port}} {{.Host}}:{{.Port}} check port {{.Port}} observe layer7	
{{end}}

{{end}}
		`))
)
