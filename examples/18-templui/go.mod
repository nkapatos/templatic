module examples/templui

go 1.27.4

require (
	github.com/a-h/templ v0.3.1118
	github.com/nkapatos/templatic v0.0.0
)

require github.com/templui/templui v1.13.4 // indirect

replace github.com/nkapatos/templatic => ../..

tool github.com/nkapatos/templatic/cmd/templatic
