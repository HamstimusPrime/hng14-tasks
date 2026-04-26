package main

type command struct {
	Name string
	Args []string
}

type commands struct {
	handlersMap map[string]func(command) error
}

func (c *commands) register(name string, f func(command) error) {
	c.handlersMap[name] = f
}
