module github.com/LFDT-Paladin/paladin/rpcauth/basicauth

go 1.26.0

toolchain go1.26.8

require (
	github.com/LFDT-Paladin/paladin/common/go v0.0.0-00010101000000-000000000000
	github.com/LFDT-Paladin/paladin/toolkit v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.56.0
)

require (
	github.com/LFDT-Paladin/paladin/config v0.0.0-00010101000000-000000000000 // indirect
	github.com/LFDT-Paladin/paladin/sdk/go v0.0.0-20250828150332-fbc1c1bc663b // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

replace github.com/LFDT-Paladin/paladin/toolkit => ../../toolkit/go

replace github.com/LFDT-Paladin/paladin/common/go => ../../common/go

replace github.com/LFDT-Paladin/paladin/config => ../../config

replace github.com/LFDT-Paladin/paladin/sdk/go => ../../sdk/go
