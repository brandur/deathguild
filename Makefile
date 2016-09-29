all: install test vet lint check-gofmt

check-gofmt:
	scripts/check_gofmt.sh

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

test:
	psql postgres://localhost/deathguild-test < db/structure.sql > /dev/null
	go test $(shell go list ./... | egrep -v '/vendor/')

vet:
	go vet $(shell go list ./... | egrep -v '/vendor/')
