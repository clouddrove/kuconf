package gcp

import "fmt"

var Version = "unknown"

type VersionCmd struct{}

func (v *VersionCmd) Run(program *Options) error {
	_ = program
	_, _ = fmt.Println(Version)
	return nil
}
