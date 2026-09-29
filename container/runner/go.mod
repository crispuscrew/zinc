module github.com/crispuscrew/zinc/container/runner

go 1.26.0

require (
	github.com/crispuscrew/zinc/common v0.0.0
	github.com/godbus/dbus/v5 v5.2.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/miekg/dns v1.1.73 // indirect
	github.com/quic-go/quic-go v0.63.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/crispuscrew/zinc/common => ../../common
