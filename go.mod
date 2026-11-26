module github.com/nkapatos/templatic

go 1.27.4

require (
	github.com/a-h/templ v0.3.1118
	github.com/evanw/esbuild v0.28.2
	github.com/fsnotify/fsnotify v1.10.1
	github.com/urfave/cli/v3 v3.14.0
	golang.org/x/mod v0.43.0
	golang.org/x/tools v0.53.0
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/a-h/parse v0.0.42 // indirect
	github.com/andybalholm/brotli v1.3.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cli/browser v1.3.0 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/natefinch/atomic v1.0.1 // indirect
	github.com/nishanths/exhaustive v0.17.1 // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	honnef.co/go/tools v0.8.2 // indirect
)

tool (
	github.com/a-h/templ/cmd/templ
	github.com/nishanths/exhaustive/cmd/exhaustive
	honnef.co/go/tools/cmd/staticcheck
)
