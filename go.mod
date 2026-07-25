module github.com/reinhlord/invoiceflow

go 1.26.0

// web/node_modules ships a third-party Go package (flatted). Without this it
// joins ./... and appears in this project's own build and test output.
ignore ./web/node_modules

require github.com/jackc/pgx/v5 v5.8.0

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
