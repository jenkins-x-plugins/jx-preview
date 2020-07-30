package common

import (
	"os"
)

// BinaryName the binary name to use in help docs
var BinaryName string

// TopLevelCommand the top level command name
var TopLevelCommand string

func init() {
	BinaryName = os.Getenv("BINARY_NAME")
	if BinaryName == "" {
		BinaryName = "jx remote"
	}
	TopLevelCommand = os.Getenv("TOP_LEVEL_COMMAND")
	if TopLevelCommand == "" {
		TopLevelCommand = "jx remote"
	}
}
