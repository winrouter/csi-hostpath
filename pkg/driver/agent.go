/*
Copyright © 2019 The OpenEBS Authors

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

package driver

import (
	"errors"
	"fmt"

	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/winrouter/csi-hostpath/pkg/server"
	"github.com/winrouter/csi-hostpath/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/container-storage-interface/spec/lib/go/csi"
	k8sapi "github.com/openebs/lib-csi/pkg/client/k8s"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"

	mountutils "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

// node is the server implementation
// for CSI NodeServer
type nodeServer struct {
	driver *CSIDriver
	osTool utils.OSTool
	k8smounter           *mountutils.SafeFormatAndMount
}

// NewNode returns a new instance
// of CSI NodeServer
func NewNodeServer(d *CSIDriver) csi.NodeServer {

	if d.config.ListenAddress != "" {
		exposeMetrics(d.config.ListenAddress, d.config.MetricsPath, d.config.DisableExporterMetrics)
	}

	go server.Start(server.GetLvmdPort())

	return &nodeServer{
		driver: d,
		osTool: utils.NewOSTool(),
		k8smounter: &mountutils.SafeFormatAndMount{
			Interface: mountutils.New(""),
			Exec:      utilexec.New(),
		},
	}
}

// Function to register collectors to collect LVM related metrics and exporter metrics.
//
// If disableExporterMetrics is set to false, exporter will include metrics about itself i.e (process_*, go_*).
func registerCollectors(disableExporterMetrics bool) (*prometheus.Registry, error) {
	registry := prometheus.NewRegistry()

	if !disableExporterMetrics {
		processCollector := collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})
		err := registry.Register(processCollector)
		if err != nil {
			klog.Errorf("failed to register process collector for exporter metrics collection: %s", err.Error())
			return nil, err
		}
		goProcessCollector := collectors.NewGoCollector()
		err = registry.Register(goProcessCollector)
		if err != nil {
			klog.Errorf("failed to register go process collector for exporter metrics collection: %s", err.Error())
			return nil, err
		}
	}
	//lvmCollector := collector.NewLvmCollector()
	//
	//err := registry.Register(lvmCollector)
	//if err != nil {
	//	klog.Errorf("failed to register LVM collector for LVM metrics collection: %s", err.Error())
	//	return nil, err
	//}
	return registry, nil
}

type promLog struct{}

// Implementation of Println(...) method of Logger interface of prometheus client_go.
func (p *promLog) Println(v ...interface{}) {
	klog.Error(v...)
}

func promLogger() *promLog {
	return &promLog{}
}

// Function to start HTTP server to expose LVM metrics.
//
// Parameters:
//
// listenAddr: TCP network address where the prometheus metrics endpoint will listen.
//
// metricsPath: The HTTP path where prometheus metrics will be exposed.
//
// disableExporterMetrics: Exclude metrics about the exporter itself (process_*, go_*).
func exposeMetrics(listenAddr string, metricsPath string, disableExporterMetrics bool) {

	// Registry with all the collectors registered
	registry, err := registerCollectors(disableExporterMetrics)
	if err != nil {
		klog.Fatalf("Failed to register collectors for LVM metrics collection: %s", err.Error())
	}

	http.Handle(metricsPath, promhttp.InstrumentMetricHandler(registry, promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorLog: promLogger(),
	})))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
			<head><title>LVM Exporter</title></head>
			<body>
			<h1>LVM Exporter</h1>
			<p><a href="` + metricsPath + `">Metrics</a></p>
			</body>
			</html>`))
	})

	go func() {
		if err := http.ListenAndServe(listenAddr, nil); err != nil {
			klog.Fatalf("Failed to start HTTP server at specified address (%q) and metrics path (%q) to expose LVM metrics: %s", listenAddr, metricsPath, err.Error())
		}
	}()
}


// NodePublishVolume publishes (mounts) the volume
// at the corresponding node at a given path
//
// This implements csi.NodeServer
// volume_id: yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866
// staging_target_path: /var/lib/kubelet/plugins/kubernetes.io/csi/pv/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/globalmount
// target_path: /var/lib/kubelet/pods/2a7bbb9c-c915-4006-84d7-0e3ac9d8d70f/volumes/kubernetes.io~csi/yoda-70597cb6-c08b-4bbb-8d41-c4afcfa91866/mount
func (ns *nodeServer) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {

	var (
		err error
	)

	if err = ns.validateNodePublishReq(req); err != nil {
		return nil, err
	}

	// Step 1: check
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: Volume ID not provided")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: targetPath is empty")
	}
	log.Infof("NodePublishVolume: start to mount volume %s to target path %s", volumeID, targetPath)


	switch req.GetVolumeCapability().GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		// attempt block mount operation on the requested path
		err = nil
	case *csi.VolumeCapability_Mount:
		// attempt filesystem mount operation on the requested path
		// check targetPath
		if _, err := ns.osTool.Stat(targetPath); os.IsNotExist(err) {
			if err := ns.osTool.MkdirAll(targetPath, 0750); err != nil {
				return &csi.NodePublishVolumeResponse{}, fmt.Errorf("mountLvmFS: fail to mkdir target path %s: %s", targetPath, err.Error())
			}
		}
		err := ns.mountMountPointVolume(ctx, req)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodePublishVolume: fail to mount mountpoint volume %s with path %s: %s", volumeID, targetPath, err.Error())
		}
	}

	if err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}


func (ns *nodeServer) mountMountPointVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	sourcePath := ""
	targetPath := req.TargetPath
	if value, ok := req.VolumeContext[string(pkg.MPName)]; ok {
		sourcePath = value
	}
	if sourcePath == "" {
		return fmt.Errorf("mountMountPointVolume: sourcePath of volume %s is empty", req.VolumeId)
	}

	notmounted, err := ns.k8smounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return fmt.Errorf("mountMountPointVolume: check if targetPath %s is mounted: %s", targetPath, err.Error())
	}
	if !notmounted {
		log.Infof("mountMountPointVolume: volume %s(%s) is already mounted", req.VolumeId, targetPath)
		return nil
	}

	// start to mount
	mnt := req.VolumeCapability.GetMount()
	options := append(mnt.MountFlags, "bind")
	if req.Readonly {
		options = append(options, "ro")
	}
	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}
	log.Infof("mountMountPointVolume: mount volume %s to %s with flags %v and fsType %s", req.VolumeId, targetPath, options, fsType)
	if err = ns.k8smounter.Mount(sourcePath, targetPath, fsType, options); err != nil {
		return fmt.Errorf("mountMountPointVolume: fail to mount %s to %s: %s", sourcePath, targetPath, err.Error())
	}
	return nil
}

// NodeUnpublishVolume unpublishes (unmounts) the volume
// from the corresponding node from the given path
//
// This implements csi.NodeServer
func (ns *nodeServer) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {

	var (
		err error
	)

	if err = ns.validateNodeUnpublishReq(req); err != nil {
		return nil, err
	}

	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	if err := ns.osTool.CleanupMountPoint(targetPath, ns.k8smounter, true /*extensiveMountPointCheck*/); err != nil {
		return nil, status.Errorf(codes.Internal, "NodeUnpublishVolume: fail to umount volume %s for path %s: %s", volumeID, targetPath, err.Error())
	}

	klog.Infof("lvm: volume %s path: %s has been unmounted.",
		volumeID, targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetInfo returns node details
//
// This implements csi.NodeServer
func (ns *nodeServer) NodeGetInfo(
	ctx context.Context,
	req *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {

	node, err := k8sapi.GetNode(ns.driver.config.NodeID)
	if err != nil {
		klog.Errorf("failed to get the node %s", ns.driver.config.NodeID)
		return nil, err
	}
	/*
	 * The driver will support all the keys and values defined in the node's label.
	 * if nodes are labeled with the below keys and values
	 * map[beta.kubernetes.io/arch:amd64 beta.kubernetes.io/os:linux kubernetes.io/arch:amd64 kubernetes.io/hostname:pawan-node-1 kubernetes.io/os:linux node-role.kubernetes.io/worker:true openebs.io/zone:zone1 openebs.io/zpool:ssd]
	 * The driver will support below key and values
	 * {
	 *	beta.kubernetes.io/arch:amd64
	 *	beta.kubernetes.io/os:linux
	 *	kubernetes.io/arch:amd64
	 *	kubernetes.io/hostname:pawan-node-1
	 *	kubernetes.io/os:linux
	 *	node-role.kubernetes.io/worker:true
	 *	openebs.io/zone:zone1
	 *	openebs.io/zpool:ssd
	 * }
	 */

	// add driver's topology key
	topology := map[string]string{
		"kubernetes.io/hostname": ns.driver.config.NodeID,
	}

	// support topologykeys from env ALLOWED_TOPOLOGIES
	allowedTopologies := os.Getenv("ALLOWED_TOPOLOGIES")
	allowedKeys := strings.Split(allowedTopologies, ",")
	for _, key := range allowedKeys {
		if key != "" {
			v, ok := node.Labels[key]
			if ok {
				topology[key] = v
			}
		}
	}

	return &csi.NodeGetInfoResponse{
		NodeId: ns.driver.config.NodeID,
		AccessibleTopology: &csi.Topology{
			Segments: topology,
		},
	}, nil
}

// NodeGetCapabilities returns capabilities supported
// by this node service
//
// This implements csi.NodeServer
func (ns *nodeServer) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
					},
				},
			},
		},
	}, nil
}

// TODO
// This needs to be implemented
//
// NodeStageVolume mounts the volume on the staging
// path
//
// This implements csi.NodeServer
func (ns *nodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {

	volName := strings.ToLower(req.GetVolumeId())
	klog.Infof("NodeStageVolume volume %s", volName)

	// prepare hostpath

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unmounts the volume from
// the staging path
//
// This implements csi.NodeServer
func (ns *nodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	volName := strings.ToLower(req.GetVolumeId())
	klog.Infof("NodeUnstageVolume volume %s", volName)

	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// TODO
// Verify if this needs to be implemented
//
// # NodeExpandVolume resizes the filesystem if required
//
// If ControllerExpandVolumeResponse returns true in
// node_expansion_required then FileSystemResizePending
// condition will be added to PVC and NodeExpandVolume
// operation will be queued on kubelet
//
// This implements csi.NodeServer
func (ns *nodeServer) NodeExpandVolume(
	ctx context.Context,
	req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if req.GetVolumePath() == "" || volumeID == "" {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"path not provided for NodeExpandVolume Request %s",
			volumeID,
		)
	}

	return &csi.NodeExpandVolumeResponse{
		CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
	}, nil
}

// NodeGetVolumeStats returns statistics for the
// given volume
func (ns *nodeServer) NodeGetVolumeStats(
	ctx context.Context,
	req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	var usage []*csi.VolumeUsage
	volumeAbnormal := false
	volumeErrMsg := ""

	volID := req.GetVolumeId()
	path := req.GetVolumePath()

	if len(volID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume id is not provided")
	}
	if len(path) == 0 {
		return nil, status.Error(codes.InvalidArgument, "path is not provided")
	}


	var sfs unix.Statfs_t
	if err := unix.Statfs(path, &sfs); err != nil {
		return nil, status.Errorf(codes.Internal, "statfs on %s failed: %v", path, err)
	}

	usage = append(usage, &csi.VolumeUsage{
		Unit:      csi.VolumeUsage_BYTES,
		Total:     int64(sfs.Blocks) * int64(sfs.Bsize),
		Used:      int64(sfs.Blocks-sfs.Bfree) * int64(sfs.Bsize),
		Available: int64(sfs.Bavail) * int64(sfs.Bsize),
	})
	usage = append(usage, &csi.VolumeUsage{
		Unit:      csi.VolumeUsage_INODES,
		Total:     int64(sfs.Files),
		Used:      int64(sfs.Files - sfs.Ffree),
		Available: int64(sfs.Ffree),
	})

	return &csi.NodeGetVolumeStatsResponse{Usage: usage, VolumeCondition: &csi.VolumeCondition{Abnormal: volumeAbnormal, Message: volumeErrMsg}}, nil
}

func (ns *nodeServer) validateNodePublishReq(
	req *csi.NodePublishVolumeRequest,
) error {
	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument,
			"Volume capability missing in request")
	}

	if len(req.GetVolumeId()) == 0 {
		return status.Error(codes.InvalidArgument,
			"Volume ID missing in request")
	}
	return nil
}

func (ns *nodeServer) validateNodeUnpublishReq(
	req *csi.NodeUnpublishVolumeRequest,
) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument,
			"Volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument,
			"Target path missing in request")
	}
	return nil
}
