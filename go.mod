module github.com/opencord/openolt-scale-tester

go 1.16

replace (
	github.com/coreos/bbolt v1.3.4 => go.etcd.io/bbolt v1.3.4
	go.etcd.io/bbolt v1.3.4 => github.com/coreos/bbolt v1.3.4
	google.golang.org/grpc => google.golang.org/grpc v1.25.1
)

require (
	github.com/cenkalti/backoff/v3 v3.1.1
	github.com/golang/protobuf v1.5.2
	github.com/opencord/voltha-lib-go/v7 v7.0.6
	github.com/opencord/voltha-protos/v5 v5.0.2
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e
	google.golang.org/grpc v1.41.0
)
