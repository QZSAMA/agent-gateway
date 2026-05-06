#!/bin/bash
cd "$(dirname "$0")/../src"
go test -v ./test_integration/
