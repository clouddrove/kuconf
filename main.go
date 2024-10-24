package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/clouddrove/kuconf/program/aws"
	"github.com/clouddrove/kuconf/program/gcp"
	"github.com/rs/zerolog/log"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide (aws or gcp) as the first argument.")
		os.Exit(1)
	}

	platform := os.Args[1]

	var optionsGCP gcp.Options
	var optionsAWS aws.Options

	var ctx *kong.Context
	var err error

	switch platform {
	case "gcp":
		ctx, err = optionsGCP.Parse(os.Args[2:])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := ctx.Run(&optionsGCP); err != nil {
			log.Err(err).Msg("Program failed for GCP")
			os.Exit(1)
		}

	case "aws":
		ctx, err = optionsAWS.Parse(os.Args[2:])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := ctx.Run(&optionsAWS); err != nil {
			log.Err(err).Msg("Program failed for AWS")
			os.Exit(1)
		}

	default:
		fmt.Println("Invalid cloud provider. Please choose either 'aws' or 'gcp'.")
		os.Exit(1)
	}
}
