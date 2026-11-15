package main

import (
	"os"
	"tb_live_module/internal/backendcli"
)

func main() { os.Exit(backendcli.Run(os.Args[1:], os.Stdout)) }
