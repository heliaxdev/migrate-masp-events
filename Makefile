BIN     := migrate-masp-events
RUSTLIB := libparse.a

.PHONY: all
all: $(BIN)

.PHONY: fmt
fmt:
	go fmt github.com/heliaxdev/migrate-masp-events
	go fmt github.com/heliaxdev/migrate-masp-events/namada
	go fmt github.com/heliaxdev/migrate-masp-events/proto/types

$(BIN): $(RUSTLIB)
	env CGO_ENABLED=1 go build

$(RUSTLIB):
	cd namada/parse && cargo build --release
