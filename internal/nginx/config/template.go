package config

const mainTemplate = `
user  nginx;
worker_processes  auto;

error_log stderr debug;
pid /etc/nginx/nginx.pid;

load_module /usr/lib/nginx/modules/ngx_http_js_module.so;

events {
    worker_connections  1024;
}


http {
    # include       /etc/nginx/mime.types;
    default_type  application/octet-stream;

    log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
                      '$status $body_bytes_sent "$http_referer" '
                      '"$http_user_agent" "$http_x_forwarded_for"';

    access_log  stdout  main;

	js_import /usr/lib/nginx/modules/njs/httpmatches.js;

    server_names_hash_bucket_size 256;
	server_names_hash_max_size 1024;

    sendfile        on;
    #tcp_nopush     on;

    keepalive_timeout  65;

    #gzip  on;

	#############
	# upstreams #
	#############

	{{ range $u := .Upstreams }}
	upstream {{ $u.Name }} {
		random two least_conn;
		zone {{ $u.Name }} 512k;
		{{ range $server := $u.Servers }} 
		server {{ $server.Address }};
		{{- end }}
	}
	{{ end }}

	#################
	# split clients #
	#################

	{{ range $sc := .SplitClients }}
	split_clients $request_id ${{ $sc.VariableName }} {
		{{- range $d := $sc.Distributions }}
			{{- if eq $d.Percent "0.00" }}
		# {{ $d.Percent }}% {{ $d.Value }};
			{{- else }}
		{{ $d.Percent }}% {{ $d.Value }};
			{{- end }}
		{{- end }}
	}
	{{ end }}

	###########	
	# servers #
	###########

	{{ range $s := .Servers -}}
		{{ if $s.IsDefaultSSL -}}
	server {
		listen 443 ssl default_server;

		ssl_reject_handshake on;
	}
		{{- else if $s.IsDefaultHTTP }}
	server {
		listen 80 default_server;

		default_type text/html;
		return 404;
	}
		{{- else }}
	server {
			{{- if $s.SSL }}
		listen 443 ssl;
		ssl_certificate {{ $s.SSL.Certificate }};
		ssl_certificate_key {{ $s.SSL.CertificateKey }};

		if ($ssl_server_name != $host) {
			return 421;
		}
			{{- end }}

		server_name {{ $s.ServerName }};

			{{ range $l := $s.Locations }}
		location {{ if $l.Exact }}= {{ end }}{{ $l.Path }} {
			{{ if $l.Internal -}}
			internal;
			{{ end }}

			{{- if $l.Return -}}
			return {{ $l.Return.Code }} "{{ $l.Return.Body }}";
			{{ end }}

			{{- if $l.HTTPMatchVar -}}
			set $http_matches {{ $l.HTTPMatchVar | printf "%q" }};
			js_content httpmatches.redirect;
			{{ end }}

			{{- if $l.ProxyPass -}}
			proxy_set_header Host $host;
			proxy_pass {{ $l.ProxyPass }}$request_uri;
			{{- end }}
		}
			{{ end }}
	}
		{{- end }}
	{{ end }}
	server {
		listen unix:/var/lib/nginx/nginx-502-server.sock;
		access_log off;

		return 502;
	}

	server {
		listen unix:/var/lib/nginx/nginx-500-server.sock;
		access_log off;
		
		return 500;
	}
}
`
