module github.com/book-expert/pdf-to-png-service

go 1.25.5

require (
	github.com/book-expert/common-events v0.0.3
	github.com/book-expert/common-worker v0.0.3
	github.com/book-expert/logger v0.1.4
	github.com/gen2brain/go-fitz v1.2.0
	github.com/google/uuid v1.6.0
	github.com/nats-io/nats.go v1.46.1
	github.com/pelletier/go-toml/v2 v2.2.4
)

require (
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
)

replace (
	github.com/book-expert/common-events v0.0.3 => ../common-events
	github.com/book-expert/common-worker v0.0.3 => ../common-worker
)
