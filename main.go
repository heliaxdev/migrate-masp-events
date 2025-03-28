package main

func main() {
	subCommands := make(map[string]*SubCommand)

	RegisterCommandPrintEvents(subCommands)
	RegisterCommandPrintBlocks(subCommands)
	RegisterCommandMigrate(subCommands)

	r := NewRunner(subCommands)
	r.Run()
}
