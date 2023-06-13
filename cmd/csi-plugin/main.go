/*
Copyright © 2020 The Kubernetes Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/winrouter/csi-hostpath/pkg/config"
	"github.com/winrouter/csi-hostpath/pkg/driver"
	"github.com/winrouter/csi-hostpath/pkg/version"
)


func main() {
	_ = flag.CommandLine.Parse([]string{})
	var config = config.Default()

	cmd := &cobra.Command{
		Use:   "lvm-driver",
		Short: "driver for provisioning lvm volume",
		Long: `provisions and deprovisions the volume
		    on the node which has volume group configured.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(config)
		},
	}

	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	cmd.PersistentFlags().StringVar(
		&config.NodeID, "nodeid", lvm.NodeID, "NodeID to identify the node running this driver",
	)

	cmd.PersistentFlags().StringVar(
		&config.Version, "version", "", "Displays driver version",
	)

	cmd.PersistentFlags().StringVar(
		&config.Endpoint, "endpoint", "unix://csi/csi.sock", "CSI endpoint",
	)

	cmd.PersistentFlags().StringVar(
		&config.DriverName, "name", "local.csi.openebs.io", "Name of this driver",
	)

	cmd.PersistentFlags().StringVar(
		&config.PluginType, "plugin", "csi-plugin", "Type of this driver i.e. controller or node",
	)

	cmd.PersistentFlags().BoolVar(
		&config.SetIOLimits, "setiolimits", false,
		"Whether to set iops, bps rate limit for pods accessing volumes",
	)

	cmd.PersistentFlags().StringVar(
		&config.ContainerRuntime, "container-runtime", "containerd",
		"Whether to set iops, bps rate limit for pods accessing volumes",
	)

	cmd.PersistentFlags().StringVar(
		&config.ListenAddress, "listen-address", "", "The TCP network address where the prometheus metrics endpoint will listen (example: `:9080`). The default is empty string, which means metrics endpoint is disabled.",
	)

	cmd.PersistentFlags().StringVar(
		&config.MetricsPath, "metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.",
	)

	cmd.PersistentFlags().BoolVar(
		&config.DisableExporterMetrics, "disable-exporter-metrics", true, "Exclude metrics about the exporter itself (process_*, go_*).",
	)

	cmd.PersistentFlags().IntVar(
		&config.KubeAPIQPS, "kube-api-qps", 0, "QPS to use while talking with Kubernetes API server.",
	)

	cmd.PersistentFlags().IntVar(
		&config.KubeAPIBurst, "kube-api-burst", 0, "Burst to allow while talking with Kubernetes API server.",
	)

	config.RIopsLimitPerGB = cmd.PersistentFlags().StringSlice(
		"riops-per-gb", []string{},
		"Read IOPS per GB limit to use for each volume group prefix, "+
			"--riops-per-gb=\"vg1-prefix:100,vg2-prefix:200\"",
	)

	config.WIopsLimitPerGB = cmd.PersistentFlags().StringSlice(
		"wiops-per-gb", []string{},
		"Write IOPS per GB limit to use for each volume group prefix, "+
			"--wiops-per-gb=\"vg1-prefix:100,vg2-prefix:200\"",
	)

	config.RBpsLimitPerGB = cmd.PersistentFlags().StringSlice(
		"rbps-per-gb", []string{},
		"Read BPS per GB limit to use for each volume group prefix, "+
			"--rbps-per-gb=\"vg1-prefix:100,vg2-prefix:200\"",
	)

	config.WBpsLimitPerGB = cmd.PersistentFlags().StringSlice(
		"wbps-per-gb", []string{},
		"Write BPS per GB limit to use for each volume group prefix, "+
			"--wbps-per-gb=\"vg1-prefix:100,vg2-prefix:200\"",
	)

	err := cmd.Execute()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s", err.Error())
		os.Exit(1)
	}
}

func run(config *config.Config) {
	if config.Version == "" {
		config.Version = version.Current()
	}

	klog.Infof("LVM Driver Version :- %s - commit :- %s", version.Current(), version.GetGitCommit())
	klog.Infof(
		"DriverName: %s Plugin: %s EndPoint: %s NodeID: %s SetIOLimits: %v ContainerRuntime: %s RIopsPerGB: %v WIopsPerGB: %v RBpsPerGB: %v WBpsPerGB: %v",
		config.DriverName,
		config.PluginType,
		config.Endpoint,
		config.NodeID,
		config.SetIOLimits,
		config.ContainerRuntime,
		*config.RIopsLimitPerGB,
		*config.WIopsLimitPerGB,
		*config.RBpsLimitPerGB,
		*config.WBpsLimitPerGB,
	)



	err := driver.New(config).Run()
	if err != nil {
		log.Fatalln(err)
	}
	os.Exit(0)
}