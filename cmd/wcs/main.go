package main

import (
	"github.com/opencloud-eu/woodpecker-ci-config-service"
	"github.com/opencloud-eu/woodpecker-ci-config-service/internal/cmd"
)

func main() {
	wcs.Must(cmd.Execute())
}
