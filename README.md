# migrate-masp-events

Tool developed to migrate MASP events produced by a Namada node
on version [`v1.1.4`](https://github.com/anoma/namada/releases/tag/v1.1.4)
to the event format produced by a node on the next release. Events should be
migrated by node operators who intend to expose their RPC servers to
MASP indexers.

## Build instructions

Simply run `make`. You will need recent Go and Rust toolchains
available in your `PATH`. The resulting binary is `migrate-masp-events`.

## Usage

    $ migrate-masp-events -h
    $ migrate-masp-events <subcmd> -h

## Migration instructions

First and foremost, **make sure you are not running the migrations
from a pruned CometBFT node** (such as a state synced node). This is
important, as the migrations require access to historical tx data.

1. Ensure the Namada node on version `v1.1.4` that is going to
   be migrated is not running
2. Create a backup of the CometBFT `data` directory
    - In practice, only the `state.db` needs to be backed up,
      but there is no harm in creating a backup of the entire
      directory structure
3. Locate a MASP indexer webserver that is still running `v1.2.0`
    - Its http endpoint will look something like <https://masp.indexer/api/v1>
4. Run the events migration software
    ```
    $ migrate-masp-events migrate -cometbft-homedir path/to/cometbft/home -masp-indexer https://masp.indexer/api/v1
    ```

### Troubleshooting

- **Invalid masp indexer commit:** Indexers running other versions will
make the migrations error out. If your indexer is running `v1.2.1` or
some other compatible version, you may include the `-invalid-masp-commit-not-err`
flag in the command of step 4, above.
- **Connection reset by peer:** Some weaker machines may run into trouble while
migrating, by getting "connection reset by peer" errors. If that is the case,
pass the flag `-max-concurrent-requests 10` to the command from step 4, above.
- **Messed up CometBFT db:** If you happen to mess up and run the old full
node binaries after migrating, you can pass the `-continue-migrating` flag
to `migrate-masp-events migrate` in order to fix the events, with a new run
of the migrations software (i.e. repeat step 4). If CometBFT's `state.db`
ends up in a completely messed up state, you can still re-run the migrations
after restoring this database from the backup created on step 2.
