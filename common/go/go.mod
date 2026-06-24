module github.com/LFDT-Paladin/paladin/common/go

go 1.24.4

require (
	github.com/LFDT-Paladin/paladin/config v0.0.0-00010101000000-000000000000
	github.com/mattn/go-isatty v0.0.20
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.4
	github.com/stretchr/testify v1.11.1
	golang.org/x/text v0.33.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	golang.org/x/sys v0.40.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/LFDT-Paladin/paladin/config => ../../config
