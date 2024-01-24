#!/bin/bash
go mod tidy
go generate
CGO_ENABLED=0 go build -tags vfs
