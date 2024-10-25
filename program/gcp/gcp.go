package gcp

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func (program *Options) getProjects() <-chan string {
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

func (program *Options) getClustersFrom(s *gcpSessionInfo, clusters chan<- GCPClusterInfo) {
    wg := sync.WaitGroup{}
    defer wg.Wait()

    req := &containerpb.ListClustersRequest{
        Parent: "projects/" + s.project + "/locations/" + s.zone,
    }

    s.log.Debug().Str("request", req.Parent).Msg("Requesting cluster listing")

    out, err := s.session.ListClusters(context.Background(), req)
    if err != nil {
        s.log.Error().Err(err).Msg("Error listing GKE clusters")
        return
    }

    s.log.Debug().Int("number_of_clusters", len(out.Clusters)).Msg("GKE clusters found")

    if len(out.Clusters) > 0 {
        stats.Clusters.Add(int32(len(out.Clusters)))
    }

    if len(out.Clusters) == 0 {
        s.log.Warn().Msg("No GKE clusters found in the specified project and zone")
    }

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
}

func (program *Options) getUniqueGCPSessions() <-chan *gcpSessionInfo {
    sessions := make(chan *gcpSessionInfo)

    go func() {
        wg := sync.WaitGroup{}
        defer close(sessions)
        defer wg.Wait()

        projects := make(map[string]bool)
        for info := range program.getProjectSessions() {
            if _, found := projects[info.project]; found {
                info.log.Debug().Msg("Project is duplicate")
                continue
            }
            
            info.log.Debug().Msg("Project is good for use")
            projects[info.project] = true
            stats.UniqueProjects.Add(1)
            sessions <- info

            if strings.Count(info.zone, "-") == 2 {
                for _, zone := range program.Zones {
                    if zone != info.zone {
                        wg.Add(1)
                        go func(project, zone string) {
                            defer wg.Done()
                            log := log.With().Str("project", project).Str("zone", zone).Logger()
                            log.Debug().Msg("Creating regional session")
                            
                            if s, err := program.newGCPSession(project, zone); err == nil {
                                sessions <- s
                            } else {
                                log.Error().Err(err).Msg("Failed to create GCP session")
                            }
                        }(info.project, zone)
                    }
                }
            }
        }
    }()

    return sessions
}

func (program *Options) getProjectSessions() <-chan *gcpSessionInfo {

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

func (program *Options) newGCPSession(project, zone string) (*gcpSessionInfo, error) {
    opts := []option.ClientOption{}
    if program.CredentialsFile != "" {
        opts = append(opts, option.WithCredentialsFile(program.CredentialsFile))
    }
    
    sess, err := container.NewClusterManagerClient(context.Background(), opts...)
    if err != nil {
        return nil, err
    }

    stats.Projects.Add(1)
    stats.UsableProjects.Add(1)
    
    logger := log.With().Str("project", project).Str("zone", zone).Logger()
    logger.Debug().Msg("GCP project session created")
    
    return &gcpSessionInfo{
        project: project,
        zone:    zone,
        session: sess,
        log:     logger,
    }, nil
}

func NewGCPSession(project, zone string, log zerolog.Logger) (*gcpSessionInfo, error) {
    opts := []option.ClientOption{}
    if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
        opts = append(opts, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
    }
    
    sess, err := container.NewClusterManagerClient(context.Background(), opts...)
    if err != nil {
        log.Error().Err(err).Msg("Failed to create GCP ClusterManagerClient")
        return nil, err
    }

    log.Debug().Msg("GCP project session created successfully")

    return &gcpSessionInfo{
        project: project,
        zone:    zone,
        session: sess,
        log:     log,
    }, nil

}