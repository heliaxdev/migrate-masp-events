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
	go build

$(RUSTLIB):
	cd namada/parse && cargo build --release
