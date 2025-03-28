BIN     := migrate-masp-events
RUSTLIB := libparse.a

.PHONY: all
all: $(BIN)

$(BIN): $(RUSTLIB)
	go build

$(RUSTLIB):
	cd namada/parse && cargo build --release
