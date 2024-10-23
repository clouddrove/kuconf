package program

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/container/apiv1"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"google.golang.org/api/option"
)

type gcpSessionInfo struct {
	project string
	zone    string
	session *container.ClusterManagerClient
	log     zerolog.Logger
}

type GCPClusterInfo struct {
	*containerpb.Cluster
	log     zerolog.Logger
	session *gcpSessionInfo
}

type Options2 struct {
	Projects    []string
	ProjectFile string
	Zones       []string
}

func (program *Options2) getProjects() <-chan string {
	output := make(chan string)

	if len(program.Projects) < 1 {
		go func() {
			defer close(output)
			if f, err := os.Open(program.ProjectFile); err == nil {
				scanner := bufio.NewScanner(f)
				scanner.Split(bufio.ScanLines)

				for scanner.Scan() {
					s := strings.TrimSpace(scanner.Text())
					if s != "" {
						output <- s
					}
				}

			} else {
				log.Error().Str("file", program.ProjectFile).Err(err).Msg("Failed to open project file")
			}
		}()
	} else {
		go func() {
			defer close(output)
			for _, p := range program.Projects {
				output <- p
			}
		}()
	}

	return output
}

func (program *Options2) getClustersFrom(s *gcpSessionInfo, clusters chan<- GCPClusterInfo) {
	wg := sync.WaitGroup{}
	defer wg.Wait()

	req := &containerpb.ListClustersRequest{
		Parent: "projects/" + s.project + "/locations/" + s.zone,
	}

	s.log.Debug().Msg("Listing GKE clusters")
	if out, err := s.session.ListClusters(context.Background(), req); err == nil {
		s.log.Debug().Msg("Getting GKE Clusters")

		for _, c := range out.Clusters {
			wg.Add(1)
			go func(c *containerpb.Cluster) {
				defer wg.Done()
				s.log.Debug().Str("cluster_name", c.Name).Msg("Found GKE cluster")

				clusters <- GCPClusterInfo{
					Cluster: c,
					log:     s.log.With().Str("cluster_name", c.Name).Logger(),
					session: s,
				}
			}(c)
		}
	} else {
		s.log.Error().Err(err).Msg("Error listing GKE clusters")
	}
}

func (program *Options2) getUniqueGCPSessions() <-chan *gcpSessionInfo {

	sessions := make(chan *gcpSessionInfo)

	go func() {
		wg := sync.WaitGroup{}

		defer close(sessions)
		defer wg.Wait()

		projects := make(map[string]bool)
		for info := range program.getProjectSessions() {
			if _, found := projects[info.project]; found {
				info.log.Debug().Msg("Project is duplicate")
			} else {
				info.log.Debug().Msg("Project is good for use")
				projects[info.project] = true

				sessions <- info

				for _, zone := range program.Zones {
					wg.Add(1)
					go func(project, zone string) {
						defer wg.Done()
						if zone != info.zone {
							log := log.With().Str("project", info.project).Str("zone", zone).Logger()
							log.Debug().Msg("Creating regional session")
							if s, err := container.NewClusterManagerClient(context.Background(), option.WithCredentialsFile("path/to/credentials.json")); err == nil {
								sessions <- &gcpSessionInfo{
									project: project,
									zone:    zone,
									session: s,
									log:     log,
								}
							} else {
								log.Error().Err(err).Msg("Failed to create GCP session")
							}
						}
					}(info.project, zone)
				}
			}
		}
	}()

	return sessions
}

func (program *Options2) getProjectSessions() <-chan *gcpSessionInfo {

	sessions := make(chan *gcpSessionInfo)
	wg := sync.WaitGroup{}

	go func() {
		defer close(sessions)
		defer wg.Wait()

		projects := program.getProjects()

		for p := range projects {
			log := log.With().Str("project", p).Str("zone", program.Zones[0]).Logger()
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				if s, err := NewGCPSession(p, program.Zones[0], log); err == nil {
					sessions <- s
				}
			}(p)
		}
	}()

	return sessions
}

func NewGCPSession(project, zone string, log zerolog.Logger) (*gcpSessionInfo, error) {
	if sess, err := container.NewClusterManagerClient(context.Background(), option.WithCredentialsFile("path/to/credentials.json")); err == nil {
		log.Debug().Msg("GCP project session created")
		return &gcpSessionInfo{
			project: project,
			zone:    zone,
			session: sess,
			log:     log,
		}, nil
	} else {
		return nil, err
	}
}
