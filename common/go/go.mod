module github.com/LFDT-Paladin/paladin/common/go

go 1.26.0

toolchain go1.26.8

require (
	github.com/LFDT-Paladin/paladin/config v0.0.0-00010101000000-000000000000
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.12.1
	go.uber.org/zap v1.28.0
	golang.org/x/text v0.41.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/docker/go-units v0.5.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

replace github.com/LFDT-Paladin/paladin/config => ../../config
