package main

import (
	"os"
	"tb_live_module/internal/anchorhomecli"
)

func main() { os.Exit(anchorhomecli.Run(os.Args[1:], os.Stdout)) }
