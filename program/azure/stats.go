package azure

import (
	"github.com/rs/zerolog/log"
	"sync/atomic"
)

type Stats struct {
	Subscriptions, UniqueSubscriptions, UsableSubscriptions, Locations, Clusters, Errors atomic.Int32
}

func (s *Stats) Log() {
	log.Info().
		Int32("subscriptions", s.Subscriptions.Load()).
		Int32("unique_subscriptions", s.UniqueSubscriptions.Load()).
		Int32("usable_subscriptions", s.UsableSubscriptions.Load()).
		Int32("locations", s.Locations.Load()).
		Int32("clusters", s.Clusters.Load()).
		Int32("fatal_errors", s.Errors.Load()).
		Msg("Statistics")
}

var stats Stats
