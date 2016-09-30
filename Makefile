all: clean install test vet lint check-gofmt

build:
	$(GOPATH)/bin/dg-build

check-gofmt:
	scripts/check_gofmt.sh

clean:
	mkdir -p public/
	rm -f -r public/*

install:
	go install $(shell go list ./... | egrep -v '/vendor/')

# Note that unfortunately Golint doesn't work like other Go commands: it only
# takes only a single argument at a time and expects that each is the name of a
# local directory (as opposed to a package).
#
# The exit 255 trick ensures that xargs will actually bubble a failure back up
# to the entire command.
lint:
	go list ./... | egrep -v '/vendor/' | sed "s|^github\.com/brandur/sorg|.|" | xargs -I{} -n1 sh -c '$(GOPATH)/bin/golint -set_exit_status {} || exit 255'

serve:
	$(GOPATH)/bin/dg-serve

# Read from env or fall back.
TEST_DATABASE_URL ?= postgres://localhost/deathguild-test

test:
	psql $(TEST_DATABASE_URL) < db/structure.sql > /dev/null
	go test $(shell go list ./... | egrep -v '/vendor/')

vet:
	go vet $(shell go list ./... | egrep -v '/vendor/')

watch:
	fswatch -o content/ layouts/ pages/ views/ | xargs -n1 -I{} make build

# This is designed to be compromise between being explicit and readability. We
# can allow the find to discover everything in vendor/, but then the fswatch
# invocation becomes a huge unreadable wall of text that gets dumped into the
# shell. Instead, find all our own *.go files and then just tack the vendor/
# directory on separately (fswatch will watch it recursively).
GO_FILES := $(shell find . -type f -name "*.go" ! -path "./vendor/*")

# We recompile our Go source when a file changes, but we also rebuild the site
# because a change in source may have affected the build formula.
watch-go:
	fswatch -o $(GO_FILES) vendor/ | xargs -n1 -I{} make install build
