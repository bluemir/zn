package main

import (
	"os"

	"github.com/sirupsen/logrus"

	"github.com/bluemir/be/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		logrus.Fatalf("%+v", err)
		os.Exit(1)
	}
}
