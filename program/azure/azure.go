package azure

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type azureSessionInfo struct {
	subscription string
	location     string
	client       *armcontainerservice.ManagedClustersClient
	log          zerolog.Logger
}

type AzureClusterInfo struct {
	*armcontainerservice.ManagedCluster
	log     zerolog.Logger
	session *azureSessionInfo
}

func (program *Options) getSubscriptions() <-chan string {
	output := make(chan string)

	if len(program.Subscriptions) < 1 {
		go func() {
			defer close(output)
			if f, err := os.Open(program.SubscriptionFile); err == nil {
				scanner := bufio.NewScanner(f)
				scanner.Split(bufio.ScanLines)

				for scanner.Scan() {
					s := strings.TrimSpace(scanner.Text())
					if s != "" {
						output <- s
					}
				}
			} else {
				log.Error().Str("file", program.SubscriptionFile).Err(err).Msg("Failed to open subscription file")
			}
		}()
	} else {
		go func() {
			defer close(output)
			for _, s := range program.Subscriptions {
				output <- s
			}
		}()
	}

	return output
}

func (program *Options) getClustersFrom(s *azureSessionInfo, clusters chan<- AzureClusterInfo) {
	var wg sync.WaitGroup
	defer wg.Wait()

	client := s.client
	ctx := context.Background()
	uniqueClusters := make(map[string]struct{})

	pager := client.NewListPager(nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			s.log.Error().Err(err).Msg("Error listing AKS clusters")
			return
		}

		for _, c := range page.Value {
			clusterKey := fmt.Sprintf("%s-%s", *c.Name, *c.Location)
			if _, exists := uniqueClusters[clusterKey]; !exists {
				uniqueClusters[clusterKey] = struct{}{}
				stats.Clusters.Add(1)

				wg.Add(1)
				go func(c *armcontainerservice.ManagedCluster) {
					defer wg.Done()

					s.log.Debug().
						Str("cluster_name", *c.Name).
						Str("location", *c.Location).
						Msg("Found unique AKS cluster")

					clusters <- AzureClusterInfo{
						ManagedCluster: c,
						log:            s.log.With().Str("cluster_name", *c.Name).Str("location", *c.Location).Logger(),
						session:        s,
					}
				}(c)
			}
		}
	}
}

func (program *Options) getUniqueAzureSessions() <-chan *azureSessionInfo {
	sessions := make(chan *azureSessionInfo)

	go func() {
		wg := sync.WaitGroup{}
		defer close(sessions)
		defer wg.Wait()

		subscriptions := make(map[string]bool)
		for info := range program.getSubscriptionSessions() {
			if _, found := subscriptions[info.subscription]; found {
				info.log.Debug().Msg("Subscription is duplicate")
				continue
			}

			stats.UniqueSubscriptions.Add(1)
			info.log.Debug().Msg("Subscription is good for use")
			subscriptions[info.subscription] = true
			sessions <- info

			for _, location := range program.Locations {
				if location != info.location {
					stats.Locations.Add(1)
					wg.Add(1)
					go func(subscription, location string) {
						defer wg.Done()
						log := log.With().Str("subscription", subscription).Str("location", location).Logger()
						log.Debug().Msg("Creating regional session")

						if s, err := program.newAzureSession(subscription, location); err == nil {
							sessions <- s
						} else {
							log.Error().Err(err).Msg("Failed to create Azure session")
						}
					}(info.subscription, location)
				}
			}
		}
	}()

	return sessions
}

func (program *Options) getSubscriptionSessions() <-chan *azureSessionInfo {
	sessions := make(chan *azureSessionInfo)
	wg := sync.WaitGroup{}

	go func() {
		defer close(sessions)
		defer wg.Wait()

		subscriptions := program.getSubscriptions()

		for s := range subscriptions {
			stats.Subscriptions.Add(1)
			log := log.With().Str("subscription", s).Str("location", program.Locations[0]).Logger()
			wg.Add(1)
			go func(s string) {
				defer wg.Done()
				if session, err := NewAzureSession(s, program.Locations[0], log); err == nil {
					stats.UsableSubscriptions.Add(1)
					sessions <- session
				}
			}(s)
		}
	}()

	return sessions
}

func (program *Options) newAzureSession(subscription, location string) (*azureSessionInfo, error) {
	cred, err := program.getAzureCredential()
	if err != nil {
		return nil, err
	}

	client, err := armcontainerservice.NewManagedClustersClient(subscription, cred, nil)
	if err != nil {
		return nil, err
	}

	logger := log.With().Str("subscription", subscription).Str("location", location).Logger()
	logger.Debug().Msg("Azure subscription session created")

	return &azureSessionInfo{
		subscription: subscription,
		location:     location,
		client:       client,
		log:          logger,
	}, nil
}

func (program *Options) getAzureCredential() (*azidentity.DefaultAzureCredential, error) {
	return azidentity.NewDefaultAzureCredential(nil)
}

func NewAzureSession(subscription, location string, log zerolog.Logger) (*azureSessionInfo, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Azure credential")
		return nil, err
	}

	client, err := armcontainerservice.NewManagedClustersClient(subscription, cred, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Azure client factory")
		return nil, err
	}

	log.Debug().Msg("Azure subscription session created successfully")

	return &azureSessionInfo{
		subscription: subscription,
		location:     location,
		client:       client,
		log:          log,
	}, nil
}
