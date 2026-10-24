module github.com/MakerViking/TinkerView

go 1.26.0

// NOTE: `require` directives and go.sum are maintained with `go mod tidy`, and
// are committed; the Docker build uses them as-is (-mod=readonly). Direct dependencies are deliberately
// minimal — only github.com/coder/websocket (WS hub) and gopkg.in/yaml.v3
// (config). The Docker health adapter speaks the Engine REST API over plain
// net/http and disk usage uses stdlib syscall, so no docker/docker or x/sys
// dependency tree. Frigate + Bambu MQTT will add github.com/eclipse/paho.golang.
// Everything is pure-Go so `CGO_ENABLED=0` cross-compiles to linux/arm64.

require (
	github.com/coder/websocket v1.8.14
	github.com/eclipse/paho.mqtt.golang v1.5.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
)
