package azure

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/alecthomas/kong"
	"github.com/mattn/go-colorable"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Options struct {
	Version bool `help:"Show program version"`

	KubeConfig       string   `group:"Input" short:"k" help:"Kubeconfig file" type:"path" default:"~/.kube/config"`
	Subscriptions    []string `group:"Input" help:"List of Azure subscriptions to check"`
	SubscriptionFile string   `group:"Input" help:"File containing list of Azure subscriptions" type:"path"`
	Locations        []string `group:"Input" help:"List of Azure locations to check" env:"AZURE_LOCATIONS" default:"eastus,westus,centralus,northeurope,westeurope"`
	ResourceGroups   []string `group:"Input" help:"List of Azure resource groups to check"`

	Debug        bool   `group:"Info" help:"Show debugging information"`
	OutputFormat string `group:"Info" enum:"auto,jsonl,terminal" default:"auto" help:"How to show program output (auto|terminal|jsonl)"`
	Quiet        bool   `group:"Info" help:"Be less verbose than usual"`
}

func (program *Options) Parse(args []string) (*kong.Context, error) {
	parser, err := kong.New(program,
		kong.ShortUsageOnError(),
		kong.Description("Download kubeconfigs in bulk by examining AKS clusters across multiple subscriptions and locations"),
	)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return parser.Parse(args)
}

func (program *Options) Run(options *Options) error {
	config, err := program.ReadConfig()
	if err != nil {
		log.Error().Err(err).Msg("Failed to read kubeconfig file")
		return err
	}

	clusters := make(chan AzureClusterInfo)
	wg := sync.WaitGroup{}

	for sess := range program.getUniqueAzureSessions() {
		wg.Add(1)
		go func(sess *azureSessionInfo) {
			defer wg.Done()
			program.getClustersFrom(sess, clusters)
		}(sess)
	}

	go func() {
		wg.Wait()
		close(clusters)
	}()

	for c := range clusters {
		resourceGroup := ""
		if id := *c.ManagedCluster.ID; id != "" {
			parts := strings.Split(id, "/")
			for i, part := range parts {
				if part == "resourceGroups" && i+1 < len(parts) {
					resourceGroup = parts[i+1]
					break
				}
			}
		}

		if err := captureConfig(c, resourceGroup, config); err != nil {
			stats.Errors.Add(1)
			log.Error().Err(err).Msg("Error capturing cluster configuration")
		}
	}

	if err := program.WriteConfig(config); err != nil {
		stats.Errors.Add(1)
		log.Error().
			Err(err).
			Str("file", program.KubeConfig).
			Msg("Error saving kubeconfig")
	}

	stats.Log()
	if stats.Errors.Load() > 0 {
		return errors.New("Errors encountered during run")
	}
	return nil
}

func (program *Options) AfterApply() error {
	program.initLogging()
	if len(program.Locations) < 1 {
		return errors.New("Must specify at least one location")
	}
	if len(program.Subscriptions) < 1 && program.SubscriptionFile == "" {
		return errors.New("Must specify either subscriptions or subscription file")
	}
	return nil
}

func (program *Options) initLogging() {
	if program.Version {
		fmt.Println(Version)
		os.Exit(0)
	}

	switch {
	case program.Debug:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case program.Quiet:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	var out io.Writer = os.Stdout
	if os.Getenv("TERM") == "" && runtime.GOOS == "windows" {
		out = colorable.NewColorableStdout()
	}

	if program.OutputFormat == "terminal" ||
		(program.OutputFormat == "auto" && isTerminal(os.Stdout)) {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: out})
	} else {
		log.Logger = log.Output(out)
	}

	log.Logger.Debug().
		Str("version", Version).
		Str("program", os.Args[0]).
		Msg("Starting")
}

func isTerminal(file *os.File) bool {
	if fileInfo, err := file.Stat(); err != nil {
		log.Err(err).Msg("Error running stat")
		return false
	} else {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
}
