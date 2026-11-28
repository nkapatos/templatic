module examples/forms

go 1.27.4

require (
	github.com/a-h/templ v0.3.1118
	github.com/nkapatos/templatic v0.0.0
)

require (
	github.com/a-h/parse v0.0.42 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/urfave/cli/v3 v3.14.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
)

replace github.com/nkapatos/templatic => ../..

tool github.com/nkapatos/templatic/cmd/templatic
