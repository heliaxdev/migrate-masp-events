# migrate-masp-events

Tool developed to migrate MASP events produced by a Namada node
on version [`v1.1.4`](https://github.com/anoma/namada/releases/tag/v1.1.4)
to the event format produced by a node on the next release. Events should be
migrated by node operators who intend to expose their RPC servers to
MASP indexers.

## Build instructions

Simply run `make`. You will need recent Go and Rust toolchains
available in your `PATH`. The resulting binary is `migrate-masp-events`.

## Migration instructions

First and foremost, **make sure you are not running the migrations
from a pruned CometBFT node** (such as a state synced node). This is
important, as the migrations require access to historical tx data.

1. Stop the Namada full node on `v1.1.4`
2. Create a backup of the CometBFT `data` directory
    - In practice, only the `state.db` needs to be backed up,
      but there is no harm in creating a backup of the entire
      directory structure
3. Locate a MASP indexer webserver that is still running `v1.2.1`
4. Run the events migration software
    ```
    $ migrate-masp-events migrate -cometbft-homedir path/to/cometbft/home -masp-indexer https://masp.indexer/api/v1
    ```
5. Switch to the new Namada full node binaries, and restart the node

If you happen to mess up and run the old full node binaries after migrating,
you can pass the `-continue-migrating` flag to `migrate-masp-events migrate`
in order to fix the events, with a new run of the migrations software.
