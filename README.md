# migrate-masp-events

Tool developed to migrate MASP events on full or validator nodes
produced by Namada [`v1.1.x`](https://github.com/anoma/namada/releases/tag/v1.1.5) to [`v101.0.0`](https://github.com/anoma/namada/releases/tag/v101.0.0).
Events should be migrated by node operators who intend to serve
MASP related data from their RPC servers (e.g. to be consumed by
MASP indexers).

## Build instructions

Simply run `make`. You will need recent Go and Rust toolchains
available in your `PATH`. The resulting binary is `migrate-masp-events`.

## Usage
```
$ ./migrate-masp-events -h
Usage of ./migrate-masp-events:

  ./migrate-masp-events last-state      print last block state in the blockstore db of cometbft
  ./migrate-masp-events migrate         migrate old masp events (<= namada v1.1.5) in the state db of cometbft
  ./migrate-masp-events print-blocks    print all data in the blockstore db of cometbft
  ./migrate-masp-events print-events    print all end blocks events in the state db of cometbft
```

## Migration instructions

First and foremost, **make sure you are not running the migrations
from a pruned CometBFT node** (such as a state synced node). This is
important, as the migrations require access to historical tx data.

1. Ensure the Namada node on version `v1.1.x` that is going to
   be migrated is not running
2. Create a backup of the CometBFT `data` directory
    - In practice, only the `state.db` needs to be backed up,
      but there is no harm in creating a backup of the entire
      directory structure
3. Locate a MASP indexer webserver that is still running `v1.2.0` or `v1.2.1`
    - Its http endpoint will look something like <https://masp.indexer/api/v1>
4. Run the events migration software
    ```
    $ migrate-masp-events migrate -cometbft-homedir path/to/cometbft/home -masp-indexer https://masp.indexer/api/v1
    ```

### Optimization

- **Start height:** If you happen to know a block height just before
the first MASP transaction (e.g. block 1031830 on mainnet), you might
be able to speed up the migration a bit. Pass the flag `-start <height>`
to the migration command from step 4 above.
- **Concurrent connections:** The flag `-max-concurrent-requests` controls
the number of maximum concurrent connections kept alive while fetching data
from the MASP indexer. A higher number of connections results in faster
migration speeds, but be careful with idle connections timing out (see
the troubleshooting section below).

### Troubleshooting

- **Invalid masp indexer commit:** Indexers running other versions will
make the migrations error out. If your indexer is running `v1.2.1` or
some other version compatible with `v1.2.0`, you may include the
`-invalid-masp-commit-not-err` flag in the command of step 4, above.
- **Connection reset by peer:** Some weaker machines may run into trouble while
migrating, by getting "connection reset by peer" errors. If that is the case,
pass the flag `-max-concurrent-requests 10` to the command from step 4, above.
- **Messed up CometBFT db:** If you happen to mess up and run the old full
node binaries after migrating, you can pass the `-continue-migrating` flag
to `migrate-masp-events migrate` in order to fix the events, with a new run
of the migrations software (i.e. repeat step 4). If CometBFT's `state.db`
ends up in a completely messed up state, you can still re-run the migrations
after restoring this database from the backup created on step 2.
