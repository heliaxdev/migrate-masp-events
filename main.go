package main

// TODO:
//
// - include backup/restore functionality to dump old block events
//   and load them again if something goes wrong
// - hardcode phase3 start height, to only process the events in
//   those blocks and speed up the migration

func main() {
	subCommands := make(map[string]*SubCommand)

	RegisterCommandPrint(subCommands)
	RegisterCommandMigrate(subCommands)

	r := NewRunner(subCommands)
	r.Run()
}
