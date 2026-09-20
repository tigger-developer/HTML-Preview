module github.com/tigger-developer/HTML-Preview

go 1.26.8

require (
	github.com/fsnotify/fsnotify v1.10.1
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/tdewolff/parse/v2 v2.8.16
	github.com/yuin/goldmark v1.8.6
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/net v0.58.0
	golang.org/x/sys v0.47.0
)

require github.com/dlclark/regexp2/v2 v2.2.1 // indirect

require (
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/niklasfasching/go-org v1.9.1
)

replace github.com/niklasfasching/go-org => ./third_party/go-org
