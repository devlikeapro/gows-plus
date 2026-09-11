module github.com/devlikeapro/gows

go 1.26.0

toolchain go1.26.4

require (
	github.com/Masterminds/squirrel v1.5.4
	github.com/avast/retry-go v3.0.0+incompatible
	github.com/devlikeapro/goscraper v0.0.0-20260224081950-3e513f796e42 // branch: fork-master
	github.com/golang-migrate/migrate/v4 v4.18.3
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3
	github.com/h2non/bimg v1.1.9
	github.com/jackc/pgx/v5 v5.7.6
	github.com/jmoiron/sqlx v1.4.0
	github.com/mattn/go-sqlite3 v1.14.49
	github.com/stretchr/testify v1.11.1
	github.com/u2takey/ffmpeg-go v0.5.0
	go.mau.fi/whatsmeow v0.0.0-20260721154117-8b4a8ba0d318 // find "replace" for the project below with a fork project
	google.golang.org/grpc v1.77.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/caarlos0/env/v11 v11.4.1
	github.com/gogo/protobuf v1.3.2
	github.com/jellydator/ttlcache/v3 v3.4.1
	github.com/samber/lo v1.49.1
	go.mau.fi/util v0.10.1-0.20260820140024-eb612d936fde
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/beeper/argo-go v1.1.2 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/elliotchance/orderedmap/v3 v3.1.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.23 // indirect
	github.com/petermattis/goid v0.0.0-20260816044145-ed329add6b1b // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/u2takey/go-utils v0.3.1 // indirect
	github.com/vektah/gqlparser/v2 v2.5.36 // indirect
	go.mau.fi/libsignal v0.2.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/exp v0.0.0-20260813180055-c1d0aacb2297 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260720211330-0afa2a65878a // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace go.mau.fi/whatsmeow => github.com/devlikeapro/whatsmeow v0.0.0-20260911040921-c8aa905cbceb
