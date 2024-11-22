package azure

import (
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"os"
)

func captureConfig(c AzureClusterInfo, resourceGroup string, i *api.Config) error {
	certificateData := []byte(*c.ManagedCluster.Properties.NetworkProfile.ServiceCidr)

	cluster := api.Cluster{
		Server:                   "https://" + *c.ManagedCluster.Properties.Fqdn,
		CertificateAuthorityData: certificateData,
	}

	user := api.AuthInfo{
		Exec: &api.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Command:    "azure-cli",
			Args: []string{
				"aks", "get-credentials",
				"--resource-group", resourceGroup,
				"--name", *c.ManagedCluster.Name,
			},
		},
	}

	context := api.Context{
		Cluster:  *c.ManagedCluster.Name,
		AuthInfo: *c.ManagedCluster.Name,
	}

	i.Clusters[*c.ManagedCluster.Name] = &cluster
	i.AuthInfos[*c.ManagedCluster.Name] = &user
	i.Contexts[*c.ManagedCluster.Name] = &context

	return nil
}

func (program *Options) ReadConfig() (*api.Config, error) {
	if _, err := os.Stat(program.KubeConfig); os.IsNotExist(err) {
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

	if _, err := os.Stat(program.KubeConfig); os.IsNotExist(err) {
		log.Debug().Msg("No existing config file. Copying new to config")
		return os.Rename(newFile, program.KubeConfig)
	}

	if err := os.Rename(program.KubeConfig, bakFile); err == nil {
		if e2 := os.Rename(newFile, program.KubeConfig); e2 == nil {
			return nil
		} else {
			if restoreErr := os.Rename(bakFile, program.KubeConfig); restoreErr != nil {
				return errors.Wrap(restoreErr, "Error restoring kubeconfig. Backup left in "+bakFile)
			} else {
				return errors.Wrap(e2, "Error saving new kubeconfig")
			}
		}
	} else {
		return err
	}
}
