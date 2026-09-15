module github.com/LFDT-Paladin/paladin/core

go 1.25.0

toolchain go1.25.11

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/LFDT-Paladin/paladin/common/go v0.0.0-00010101000000-000000000000
	github.com/LFDT-Paladin/paladin/config v0.0.0-00010101000000-000000000000
	github.com/go-resty/resty/v2 v2.14.0
	github.com/golang-migrate/migrate/v4 v4.17.1
	github.com/google/uuid v1.6.0
	github.com/jarcoal/httpmock v1.2.0
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.22.0
	github.com/prometheus/client_model v0.6.1
	github.com/stretchr/testify v1.11.1
	github.com/tyler-smith/go-bip39 v1.1.0
	golang.org/x/crypto v0.53.0
	golang.org/x/sync v0.22.0
	golang.org/x/text v0.40.0
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
	gorm.io/driver/postgres v1.5.9
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
	sigs.k8s.io/yaml v1.6.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.32 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

replace github.com/LFDT-Paladin/paladin/common/go => ../../common/go

replace github.com/LFDT-Paladin/paladin/sdk/go => ../../sdk/go

replace github.com/LFDT-Paladin/paladin/toolkit => ../../toolkit/go

replace github.com/LFDT-Paladin/paladin/config => ../../config

replace github.com/LFDT-Paladin/paladin/registries/static => ../../registries/static

replace github.com/LFDT-Paladin/paladin/transports/grpc => ../../transports/grpc
