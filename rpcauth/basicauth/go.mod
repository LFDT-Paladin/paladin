module github.com/LFDT-Paladin/paladin/rpcauth/basicauth

go 1.24.4

require (
	github.com/LFDT-Paladin/paladin/common/go v0.0.0-00010101000000-000000000000
	github.com/LFDT-Paladin/paladin/toolkit v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.47.0
)

require (
	github.com/LFDT-Paladin/paladin/config v0.0.0-00010101000000-000000000000 // indirect
	github.com/LFDT-Paladin/paladin/sdk/go v0.0.0-20250828150332-fbc1c1bc663b // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/grpc v1.72.1 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/LFDT-Paladin/paladin/toolkit => ../../toolkit/go

replace github.com/LFDT-Paladin/paladin/common/go => ../../common/go

replace github.com/LFDT-Paladin/paladin/config => ../../config

replace github.com/LFDT-Paladin/paladin/sdk/go => ../../sdk/go
