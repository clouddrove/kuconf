package gcp

import (
	"github.com/rs/zerolog/log"
	"sync/atomic"
)

type Stats struct {
	Projects, UniqueProjects, UsableProjects, Zones, Clusters, Errors atomic.Int32
}

func (s *Stats) Log() {
	log.Info().
		Int32("projects", s.Projects.Load()).
		Int32("unique_projects", s.UniqueProjects.Load()).
		Int32("usable_projects", s.UsableProjects.Load()).
		Int32("zones", s.Zones.Load()).
		Int32("clusters", s.Clusters.Load()).
		Int32("fatal_errors", s.Errors.Load()).
		Msg("Statistics")
}

var stats Stats