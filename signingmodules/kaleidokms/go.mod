module github.com/kaleido-io/key-manager/signingmodules/kaleidokms

go 1.24.0

toolchain go1.24.3

require (
	github.com/LFDT-Paladin/paladin/common/go v0.0.0-00010101000000-000000000000
	github.com/LFDT-Paladin/paladin/config v0.0.0-00010101000000-000000000000
	github.com/LFDT-Paladin/paladin/sdk/go v0.0.0-20250828150332-fbc1c1bc663b
	github.com/LFDT-Paladin/paladin/toolkit v0.0.0-00010101000000-000000000000
	github.com/go-resty/resty/v2 v2.16.5
	github.com/stretchr/testify v1.10.0
	golang.org/x/text v0.25.0
)

require (
	github.com/aidarkhanov/nanoid v1.0.8 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hyperledger/firefly-common v1.5.6 // indirect
	github.com/hyperledger/firefly-signer v1.1.22-0.20250527171735-c3e1c8559c15 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cobra v1.9.1 // indirect
	github.com/spf13/viper v1.20.1 // indirect
	github.com/x-cray/logrus-prefixed-formatter v0.5.2 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	golang.org/x/crypto v0.38.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250519155744-55703ea1f237 // indirect
	google.golang.org/grpc v1.72.2 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/LFDT-Paladin/paladin/toolkit => ../../toolkit/go

replace github.com/LFDT-Paladin/paladin/sdk/go => ../../sdk/go

replace github.com/LFDT-Paladin/paladin/common/go => ../../common/go

replace github.com/LFDT-Paladin/paladin/config => ../../config
