BIN     := migrate-masp-events
RUSTLIB := libparse.a

OS := $(shell uname -s)

.PHONY: all
all: $(BIN)

.PHONY: test
test: $(RUSTLIB)
	go test -v

.PHONY: clean
clean:
	rm -f $(BIN)
	cd namada/parse && cargo clean

.PHONY: fmt
fmt:
	go fmt github.com/heliaxdev/migrate-masp-events
	go fmt github.com/heliaxdev/migrate-masp-events/namada
	go fmt github.com/heliaxdev/migrate-masp-events/proto/types

$(BIN): $(RUSTLIB)
ifeq ($(OS),Linux)
	env CGO_ENABLED=1 CGO_LDFLAGS=-lm go build
else
	env CGO_ENABLED=1 go build
endif

$(RUSTLIB):
	cd namada/parse && cargo build --release
