package gcp

import (
	"encoding/base64"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"os"
)

func captureConfig(c GCPClusterInfo, i *api.Config) error {
	certificateData, err := base64.StdEncoding.DecodeString(c.MasterAuth.ClusterCaCertificate)
	if err != nil {
		c.log.Error().Err(err).Msg("Failed to decode certificate authority data from GCP")
		stats.Errors.Add(1)
		return err
	}

	cluster := api.Cluster{
		Server:                   "https://" + c.Endpoint,
		CertificateAuthorityData: certificateData,
	}

	user := api.AuthInfo{
		Exec: &api.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Command:    "gke-gcloud-auth-plugin",
			Args: []string{
				"--project",
				c.session.project,
				"--location",
				c.session.zone,
				"--cluster",
				c.Name,
			},
		},
	}

	context := api.Context{
		Cluster:  c.Name,
		AuthInfo: c.Name,
	}

	i.Clusters[c.Name] = &cluster
	i.AuthInfos[c.Name] = &user
	i.Contexts[c.Name] = &context

	return nil
}

func (program *Options) ReadConfig() (*api.Config, error) {
	if _, err := os.Stat(program.KubeConfig); err != nil {
		c := api.NewConfig()
		return c, nil
	} else {
		c, err := clientcmd.LoadFromFile(program.KubeConfig)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

func (program *Options) WriteConfig(config *api.Config) error {
	newFile := program.KubeConfig + ".tmp"
	bakFile := program.KubeConfig + ".bak"

	err := clientcmd.WriteToFile(*config, newFile)
	log := log.With().Str("kubeconfig_file", program.KubeConfig).Logger()

	if err != nil {
		return err
	}

	if _, err := os.Stat(bakFile); err == nil {
		err = os.RemoveAll(bakFile)
		if err != nil {
			return errors.Wrap(err, "Failed to remove config backup file")
		}
	}

	if _, err := os.Stat(program.KubeConfig); err != nil {
		log.Debug().Msg("No existing config file. Copying new to config")
		return os.Rename(newFile, program.KubeConfig)
	}

	if err := os.Rename(program.KubeConfig, bakFile); err == nil {
		if e2 := os.Rename(newFile, program.KubeConfig); err == nil {
			return nil
		} else {
			if err := os.Rename(bakFile, program.KubeConfig); err != nil {
				return errors.Wrap(err, "Error restoring kubeconfig. Backup left in "+bakFile)
			} else {
				return errors.Wrap(e2, "Error saving new kubeconfig")
			}
		}
	} else {
		return err
	}
}
