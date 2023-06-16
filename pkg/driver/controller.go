/*
Copyright © 2019 The Kubernetes Authors

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
	"fmt"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/winrouter/csi-hostpath/pkg/config"
	remotelib "github.com/winrouter/csi-hostpath/pkg/lib"
	"github.com/winrouter/csi-hostpath/pkg/signals"
	"github.com/winrouter/csi-hostpath/pkg/utils"
	"k8s.io/client-go/tools/cache"
	log "k8s.io/klog/v2"
	"strings"
	"time"

	"k8s.io/client-go/rest"

	k8sapi "github.com/openebs/lib-csi/pkg/client/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"

	errors "github.com/openebs/lib-csi/pkg/common/errors"
	csipayload "github.com/winrouter/csi-hostpath/pkg/response"
	corelisters "k8s.io/client-go/listers/core/v1"
	kubeinformers "k8s.io/client-go/informers"

	"github.com/winrouter/csi-hostpath/pkg/client"

)

// size constants
const (
	MB = 1000 * 1000
	GB = 1000 * 1000 * 1000
	Mi = 1024 * 1024
	Gi = 1024 * 1024 * 1024

	AnnoSelectedNode = "volume.kubernetes.io/selected-node"
	KubernetesNodeIdentityKey = "kubernetes.io/hostname"
	VolumeGroupName = "csi.io/volume-group-name"
)

// controller is the server implementation
// for CSI Controller
type controller struct {
	driver       *CSIDriver
	capabilities []*csi.ControllerServiceCapability

	indexedLabel string

	k8sClient            *kubernetes.Clientset
	snapClient  snapshot.Interface

	nodeLister corelisters.NodeLister
	podLister  corelisters.PodLister
	pvcLister  corelisters.PersistentVolumeClaimLister
	pvLister   corelisters.PersistentVolumeLister

}

// NewController returns a new instance
// of CSI controller
func NewController(d *CSIDriver) csi.ControllerServer {
	ctrl := &controller{
		driver:       d,
		capabilities: newControllerCapabilities(),
	}

	if err := ctrl.init(); err != nil {
		klog.Fatalf("init controller: %v", err)
	}

	return ctrl
}

// SupportedVolumeCapabilityAccessModes contains the list of supported access
// modes for the volume
var SupportedVolumeCapabilityAccessModes = []*csi.VolumeCapability_AccessMode{
	&csi.VolumeCapability_AccessMode{
		Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	},
}



// getRoundedCapacity rounds the capacity on 1024 base
func getRoundedCapacity(size int64) int64 {

	/*
	 * volblocksize and recordsize must be power of 2 from 512B to 1M
	 * so keeping the size in the form of Gi or Mi should be
	 * sufficient to make volsize multiple of volblocksize/recordsize.
	 */
	if size > Gi {
		return ((size + Gi - 1) / Gi) * Gi
	}

	// Keeping minimum allocatable size as 1Mi (1024 * 1024)
	return ((size + Mi - 1) / Mi) * Mi
}


func (cs *controller) init() error {
	cfg, err := k8sapi.Config().Get()
	if err != nil {
		return errors.Wrapf(err, "failed to build kubeconfig")
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to build k8s clientset")
	}
	cs.k8sClient = kubeClient
	snapClient, err := snapshot.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("fail to build snapshot clientset: %s", err.Error())
	}
	cs.snapClient = snapClient


	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(cs.k8sClient, time.Second*30)
	cs.nodeLister = kubeInformerFactory.Core().V1().Nodes().Lister()
	cs.podLister = kubeInformerFactory.Core().V1().Pods().Lister()
	cs.pvcLister = kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister()
	cs.pvLister = kubeInformerFactory.Core().V1().PersistentVolumes().Lister()

	stopCh := signals.SetupSignalHandler()
	kubeInformerFactory.Start(stopCh)
	log.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(
		stopCh,
		kubeInformerFactory.Core().V1().Nodes().Informer().HasSynced,
		kubeInformerFactory.Core().V1().Pods().Informer().HasSynced,
		kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer().HasSynced,
		kubeInformerFactory.Core().V1().PersistentVolumes().Informer().HasSynced,
	); !ok {
		log.Fatalf("failed to wait for caches to sync")
	}
	log.Info("informer sync successfully")
	return nil
}

func (cs *controller) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	volumeHandler := req.VolumeId

	client, err := GetClientFromCacheOrCreate()
	if err != nil {
		return nil, err
	}

	pv, err := client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeHandler, metav1.GetOptions{})
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to get pv")
	}


	return &csi.ControllerGetVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      pv.Spec.CSI.VolumeHandle,
			CapacityBytes: int64(pv.Size()),
		},
		Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
			VolumeCondition: &csi.VolumeCondition{Abnormal: false, Message: ""},
		},
	}, nil
}

// CreateVolume provisions a volume
func (cs *controller) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateVolumeCreateReq(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	params, err := NewVolumeParams(req.GetParameters())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument,
			"failed to parse csi volume params: %v", err)
	}

	if params.PvcName == "" || params.PvcNamespace == "" {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: pvcName(%s) or pvcNamespace(%s) can not be empty", params.PvcNamespace, params.PvcName)
	}
	pvcName := params.PvcName
	pvcNamespace := params.PvcNamespace
	pvc, err := cs.pvcLister.PersistentVolumeClaims(pvcNamespace).Get(pvcName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get pvc: %s", err.Error())
	}
	nodeName, exist := pvc.Annotations[AnnoSelectedNode]
	if !exist {
		return nil, status.Errorf(codes.Unimplemented, "CreateVolume: no annotation %s found in pvc %s. Check if volumeBindingMode of storageclass is WaitForFirstConsumer, cause we only support WaitForFirstConsumer mode", AnnoSelectedNode, utils.GetNameKey(pvcNamespace, pvcName))
	}
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume: fail to connect to node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	capSize := getRoundedCapacity(req.GetCapacityRange().GetRequiredBytes())

	vgName := params.HostPath
	volName := strings.ToLower(req.GetName())
	contentSource := req.GetVolumeContentSource()

	if contentSource != nil && contentSource.GetSnapshot() != nil {
		// create pvc from snapshot
		snapshotHandle := contentSource.GetSnapshot().SnapshotId
		klog.Infof("VolumeClone: create volume from snapshot %s\n", snapshotHandle)
		options := &client.VolOptions{}
		options.Name = volName
		options.VolumeGroup = vgName
		options.SnapshotName = snapshotHandle
		options.Size = uint64(capSize)
		var volObj *remotelib.LogicalVolume
		if volObj, err = conn.GetVolume(ctx, vgName, volName); err != nil {
			return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get lv %s from node %s: %s", volName, nodeName, err.Error())
		} else {
			if volObj == nil {
				klog.Info("CreateVolume: volume %s not found, creating volume on node %s", volName, nodeName)
				outstr, err := conn.CreateVolume(ctx, options)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "CreateVolume: fail to create lv %s(options: %v): %s", utils.GetNameKey(vgName, volName), options, err.Error())
				}
				klog.Infof("CreateLvm: create lvm %s in node %s with response %s successfully", utils.GetNameKey(vgName, volName), nodeName, outstr)
			} else {
				klog.Infof("CreateVolume: lv %s already created at node %s", volName, nodeName)
			}
		}
	} else if contentSource != nil && contentSource.GetVolume() != nil {
		// create pvc from pvc
		sourceVolID := contentSource.GetVolume().GetVolumeId()
		klog.Infof("VolumeClone: create volume from pvc %s\n", sourceVolID)
		options := &client.VolOptions{}
		options.Name = volName
		options.VolumeGroup = vgName
		options.CloneName = sourceVolID
		options.Size = uint64(capSize)

		var volObj *remotelib.LogicalVolume
		if volObj, err = conn.GetVolume(ctx, vgName, volName); err != nil {
			return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get lv %s from node %s: %s", volName, nodeName, err.Error())
		} else {
			if volObj == nil {
				klog.Info("CreateVolume: volume %s not found, creating volume on node %s", volName, nodeName)
				outstr, err := conn.CreateVolume(ctx, options)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "CreateVolume: fail to create lv %s(options: %v): %s", utils.GetNameKey(vgName, volName), options, err.Error())
				}
				klog.Infof("CreateLvm: create lvm %s in node %s with response %s successfully", utils.GetNameKey(vgName, volName), nodeName, outstr)
			} else {
				klog.Infof("CreateVolume: lv %s already created at node %s", volName, nodeName)
			}
		}

	} else {
		// normal create pvc
		klog.Infof("VolumeCreate: create volume \n")
		options := &client.VolOptions{}
		options.Name = volName
		options.VolumeGroup = vgName
		options.Size = uint64(capSize)

		var volObj *remotelib.LogicalVolume
		if volObj, err = conn.GetVolume(ctx, vgName, volName); err != nil {
			return nil, status.Errorf(codes.Internal, "CreateVolume: fail to get lv %s from node %s: %s", volName, nodeName, err.Error())
		} else {
			if volObj == nil {
				klog.Info("CreateVolume: volume %s not found, creating volume on node %s", volName, nodeName)
				outstr, err := conn.CreateVolume(ctx, options)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "CreateVolume: fail to create lv %s(options: %v): %s", utils.GetNameKey(vgName, volName), options, err.Error())
				}
				klog.Infof("CreateLvm: create lvm %s in node %s with response %s successfully", utils.GetNameKey(vgName, volName), nodeName, outstr)
			} else {
				klog.Infof("CreateVolume: lv %s already created at node %s", volName, nodeName)
			}
		}
	}


	cntx := map[string]string{
		AnnoSelectedNode: nodeName,
		VolumeGroupName: vgName,
	}
	parameters := req.GetParameters()
	for key, value := range parameters {
		cntx[key] = value
	}

	createVolumeResponse := csipayload.NewCreateVolumeResponseBuilder().
		WithName(volName).
		WithCapacity(capSize).
		WithContext(cntx).
		WithContentSource(contentSource).
		WithTopology(map[string]string{
			KubernetesNodeIdentityKey: nodeName,
		}).Build()

	return createVolumeResponse, nil
}


// DeleteVolume deletes the specified volume
func (cs *controller) DeleteVolume(
	ctx context.Context,
	req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {

	// TODO check exist snapshot before delete volume
	var err error
	if err = cs.validateDeleteVolumeReq(req); err != nil {
		return nil, err
	}

	volumeID := strings.ToLower(req.GetVolumeId())
	klog.Infof("received request to delete volume %q", volumeID)
	pv, err := cs.pvLister.Get(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get pv: %s", err.Error())
	}
	// delete volume
	nodeName := utils.GetNodeNameFromCsiPV(pv)
	if nodeName == "" {
		return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get node name of pv %s", pv.Name)
	}
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to connect to node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	vgName := utils.GetVGNameFromCsiPV(pv)
	if vgName == "" {
		klog.Warningf("DeleteVolume: delete local volume %s with empty vgName(may be hacked)", volumeID)
		return &csi.DeleteVolumeResponse{}, nil
	}

	var volObj *remotelib.LogicalVolume
	if volObj, err = conn.GetVolume(ctx, vgName, volumeID); err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			klog.Warningf("DeleteVolume: lvm volume not found, skip deleting %s", volumeID)
		} else if strings.Contains(err.Error(), "Volume group \""+vgName+"\" not found") {
			klog.Warningf("DeleteVolume: Volume group not found, skip deleting %s", volumeID)
		} else {
			return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to get lv %s: %s", volumeID, err.Error())
		}
	} else {
		if volObj != nil {
			klog.Infof("DeleteVolume: found lv %s at node %s, now deleting", utils.GetNameKey(vgName, volumeID), nodeName)
			if err := conn.DeleteVolume(ctx, vgName, volumeID); err != nil {
				return nil, status.Errorf(codes.Internal, "DeleteVolume: fail to delete lv %s: %s", volumeID, err.Error())
			}
			log.Infof("DeleteVolume: delete lv %s at node %s successfully", utils.GetNameKey(vgName, volumeID), nodeName)
		} else {
			log.Warningf("DeleteVolume: empty lv name, skip deleting %s", volumeID)
		}
	}

	return csipayload.NewDeleteVolumeResponseBuilder().Build(), nil
}

func isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) bool {
	hasSupport := func(cap *csi.VolumeCapability) bool {
		for _, c := range SupportedVolumeCapabilityAccessModes {
			if c.GetMode() == cap.AccessMode.GetMode() {
				return true
			}
		}
		return false
	}

	foundAll := true
	for _, c := range volCaps {
		if !hasSupport(c) {
			foundAll = false
		}
	}
	return foundAll
}

// TODO Implementation will be taken up later

// ValidateVolumeCapabilities validates the capabilities
// required to create a new volume
// This implements csi.ControllerServer
func (cs *controller) ValidateVolumeCapabilities(
	ctx context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID := strings.ToLower(req.GetVolumeId())
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}
	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}


	var confirmed *csi.ValidateVolumeCapabilitiesResponse_Confirmed
	if isValidVolumeCapabilities(volCaps) {
		confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volCaps}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: confirmed,
	}, nil
}

// ControllerGetCapabilities fetches controller capabilities
//
// This implements csi.ControllerServer
func (cs *controller) ControllerGetCapabilities(
	ctx context.Context,
	req *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.capabilities,
	}

	return resp, nil
}



// ControllerExpandVolume resizes previously provisioned volume
//
// This implements csi.ControllerServer
func (cs *controller) ControllerExpandVolume(
	ctx context.Context,
	req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {

	nodeExpansionRequired := true
	log.V(4).Infof("ControllerExpandVolume: called with args %+v", *req)

	// Step 1: get vgName
	volumeID := req.GetVolumeId()
	pv, err := cs.pvLister.Get(volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get pv: %s", err.Error())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get node name of pv %s: %s", pv.Name, err.Error())
	}
	vgName := utils.GetVGNameFromCsiPV(pv)
	if vgName == "" {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get vgName of pv %s", pv.Name)
	}

	// Step 2: check whether the volume can be expanded
	volSizeBytes := getRoundedCapacity(req.GetCapacityRange().GetRequiredBytes())

	// Step 3: get grpc client
	nodeName := utils.GetNodeNameFromCsiPV(pv)
	if nodeName == "" {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get node name of pv %s", pv.Name)
	}
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to get grpc client at node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	// Step 4: expand volume
	if err := conn.ExpandVolume(ctx, vgName, volumeID, uint64(volSizeBytes)); err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume: fail to expand lv %s: %s", utils.GetNameKey(vgName, volumeID), err.Error())
	}


	return csipayload.NewControllerExpandVolumeResponseBuilder().
		WithCapacityBytes(volSizeBytes).
		WithNodeExpansionRequired(nodeExpansionRequired).
		Build(), nil
}


// CreateSnapshot creates a snapshot for given volume
//
// This implements csi.ControllerServer
func (cs *controller) CreateSnapshot(
	ctx context.Context,
	req *csi.CreateSnapshotRequest,
) (*csi.CreateSnapshotResponse, error) {

	klog.Infof("CreateSnapshot volume %s for %s", req.Name, req.SourceVolumeId)

	err := validateSnapshotRequest(req)
	if err != nil {
		return nil, err
	}


	snapTimeStamp := time.Now().Unix()

	// check request
	snapshotName := req.GetName()
	if len(snapshotName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot: snapshot name not provided")
	}
	srcVolumeID := req.GetSourceVolumeId()
	if len(srcVolumeID) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "CreateSnapshot: snapshot %s volume source ID not provided", snapshotName)
	}

	// get vgName
	srcPV, err := cs.pvLister.Get(srcVolumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get pv: %s", err.Error())
	}
	vgName := utils.GetVGNameFromCsiPV(srcPV)
	if vgName == "" {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get vgName of pv %s", srcPV.Name)
	}
	log.Infof("CreateSnapshot: vg of snapshot %s is %s", snapshotName, vgName)

	// get nodeName
	nodeName := utils.GetNodeNameFromCsiPV(srcPV)
	if nodeName == "" {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get node name of pv %s", srcPV.Name)
	}

	// get grpc client
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: fail to get grpc client at node %s: %s", nodeName, err.Error())
	}
	defer conn.Close()

	var sizeBytes int64

	// create lvm snapshot
	var volObj *remotelib.LogicalVolume
	if volObj, err = conn.GetVolume(ctx, vgName, snapshotName); err != nil {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: get lvm snapshot %s failed: %s", snapshotName, err.Error())
	}
	if volObj == nil {
		log.Infof("CreateSnapshot: ro snapshot %s not found, now creating on node %s", utils.GetNameKey(vgName, snapshotName), snapshotName, nodeName)
		sizeBytes, err = conn.CreateSnapshot(ctx, vgName, snapshotName, srcVolumeID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "CreateSnapshot: create lvm snapshot %s failed: %s", snapshotName, err.Error())
		}
		log.Infof("CreateSnapshot: create ro snapshot %s successfully", snapshotName)
	} else {
		log.Infof("CreateSnapshot: lvm snapshot %s in node %s already exists", snapshotName, nodeName)
	}

	return csipayload.NewCreateSnapshotResponseBuilder().
		WithSourceVolumeID(srcVolumeID).
		WithSnapshotID(snapshotName).
		WithCreationTime(snapTimeStamp, 0).
		WithReadyToUse(true).
		WithSize(sizeBytes).
		Build(), nil
}


// DeleteSnapshot deletes given snapshot
//
// This implements csi.ControllerServer
func (cs *controller) DeleteSnapshot(
	ctx context.Context,
	req *csi.DeleteSnapshotRequest,
) (*csi.DeleteSnapshotResponse, error) {

	klog.Infof("DeleteSnapshot request for %s", req.SnapshotId)

	// check req
	snapshotID := req.GetSnapshotId()
	if len(snapshotID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteSnapshot: Snapshot ID not provided")
	}

	// get volumeID from snapshotcontent
	snapContent, err := utils.GetVolumeSnapshotContent(cs.snapClient, snapshotID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get snapContent %s error: %s", snapshotID, err.Error())
	}
	srcVolumeID := *snapContent.Spec.Source.VolumeHandle

	pv, err := cs.pvLister.Get(srcVolumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get pv: %s", err.Error())
	}
	vgName := utils.GetVGNameFromCsiPV(pv)
	if vgName == "" {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get vgName of pv %s", pv.Name)
	}
	log.Infof("DeleteSnapshot: vg of snapshot %s is %s", snapshotID, vgName)

	nodeName := utils.GetNodeNameFromCsiPV(pv)
	if nodeName == "" {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get node name of pv %s", pv.Name)
	}
	conn, err := cs.getNodeConn(nodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
	}
	defer conn.Close()

	// delete lvm snapshot
	var volObj *remotelib.LogicalVolume
	if volObj, err = conn.GetVolume(ctx, vgName, snapshotID); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get lvm snapshot %s failed: %s", snapshotID, err.Error())
	}
	if volObj != nil {
		log.Infof("DeleteSnapshot: lvm ro snapshot %s found, now deleting...", snapshotID)
		err := conn.DeleteSnapshot(ctx, vgName, snapshotID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: delete lvm snapshot %s failed: %s", snapshotID, err.Error())
		}
	} else {
		log.Infof("DeleteSnapshot: lvm snapshot %s in node %s not found, skip...", snapshotID, nodeName)
		// return immediately
		return &csi.DeleteSnapshotResponse{}, nil
	}


	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots lists all snapshots for the
// given volume
//
// This implements csi.ControllerServer
func (cs *controller) ListSnapshots(
	ctx context.Context,
	req *csi.ListSnapshotsRequest,
) (*csi.ListSnapshotsResponse, error) {
	log.Infof("list snapshots")
	entries := make([]*csi.ListSnapshotsResponse_Entry, 0)
	snapshotContents, _ := cs.getAllSnapshotContentOfLocalCSI()
	for _, content := range snapshotContents {
		lvName := *content.Spec.Source.VolumeHandle
		snapName := *content.Status.SnapshotHandle
		pvObj, err := cs.pvLister.Get(lvName)
		if err != nil {
			log.Errorf("get pv %s err: %+v", lvName, err)
			continue
		}
		vgName := pvObj.Spec.CSI.VolumeAttributes["csi.io/volume-group-name"]
		nodeName := utils.GetNodeNameFromCsiPV(pvObj)
		if nodeName == "" {
			log.Errorf("not found nodeName")
			continue
		}
		conn, err := cs.getNodeConn(nodeName)
		if err != nil {
			log.Errorf("not get node conn %s", nodeName)
			continue
		}


		// delete lvm snapshot
		var vol *remotelib.LogicalVolume
		if vol, err = conn.GetVolume(ctx, vgName, snapName); err != nil {
			log.Errorf("not getVolume  %s-%s err %v", vgName, snapName, err)
			continue
		}
		conn.Close()

		readyToUse := true
		if vol.Status != "ok" {
			readyToUse = false
		}

		entry := &csi.ListSnapshotsResponse_Entry {
			Snapshot: &csi.Snapshot{
				SizeBytes:            int64(vol.Size),
				SnapshotId:           snapName,
				SourceVolumeId:       lvName,
				CreationTime:         nil,
				ReadyToUse:           readyToUse,
			},
		}
		entries = append(entries, entry)

	}
	log.Infof("list snapshot entries %v", len(entries))
	return &csi.ListSnapshotsResponse{Entries: entries}, nil
}

// ControllerUnpublishVolume removes a previously
// attached volume from the given node
//
// This implements csi.ControllerServer
func (cs *controller) ControllerUnpublishVolume(
	ctx context.Context,
	req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ControllerPublishVolume attaches given volume
// at the specified node
//
// This implements csi.ControllerServer
func (cs *controller) ControllerPublishVolume(
	ctx context.Context,
	req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {
	log.Infof("ControllerPublishVolume: called with args %+v", *req)
	return &csi.ControllerPublishVolumeResponse{}, nil
}

// GetCapacity return the capacity of the
// given node topology segment.
//
// This implements csi.ControllerServer
func (cs *controller) GetCapacity(
	ctx context.Context,
	req *csi.GetCapacityRequest,
) (*csi.GetCapacityResponse, error) {

	return &csi.GetCapacityResponse{
		AvailableCapacity: 1024,
	}, nil
}


// ListVolumes lists all the volumes
//
// This implements csi.ControllerServer
func (cs *controller) ListVolumes(
	ctx context.Context,
	req *csi.ListVolumesRequest,
) (*csi.ListVolumesResponse, error) {
	var ventries []*csi.ListVolumesResponse_Entry


	PVsInfo, err := getAllPVsOfLocalCSI()
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to get all pvs")
	}

	for _, pv := range PVsInfo {
		lvName := pv.Name
		vgName := utils.GetVGNameFromCsiPV(&pv)
		if vgName == "" {
			log.Infof("not found valid vgname")
			continue
		}

		nodeName := utils.GetNodeNameFromCsiPV(&pv)
		if nodeName == "" {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get node name of pv %s", pv.Name)
		}
		conn, err := cs.getNodeConn(nodeName)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
		}


		// delete lvm snapshot
		var volObj *remotelib.LogicalVolume
		if volObj, err = conn.GetVolume(ctx, vgName, lvName); err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get lvm snapshot %s failed: %s", lvName, err.Error())
		}
		conn.Close()

		abnormal := false
		errMsg := ""
		if volObj.Status != "ok" {
			abnormal = true
			errMsg = "volume lost"
		}

		ventry := csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      pv.Spec.CSI.VolumeHandle,
				CapacityBytes: int64(pv.Size()),
			},
			Status: &csi.ListVolumesResponse_VolumeStatus{
				VolumeCondition: &csi.VolumeCondition{Abnormal: abnormal, Message: errMsg},
			},
		}

		ventries = append(ventries, &ventry)
	}
	return &csi.ListVolumesResponse{
		Entries: ventries,
	}, nil
}

func (cs *controller) getAllSnapshotContentOfLocalCSI() (map[string]snapshotv1api.VolumeSnapshotContent, error) {
	snapContent := make(map[string]snapshotv1api.VolumeSnapshotContent, 0)
	snapContentsList, err := cs.snapClient.SnapshotV1().VolumeSnapshotContents().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Infof("list volume snapshot contents failed:%+v", err)
		return snapContent, nil
	}

	for _, content := range snapContentsList.Items {
		if content.Spec.Driver != config.LocalProvider {
			continue
		}
		if content.Spec.VolumeSnapshotRef.Name == "" {
			continue
		}
		snapContent[content.Name] = content
	}
	log.Infof("list volume snapshot contents %v", len(snapContent))
	return snapContent, nil
}


func (cs *controller) GroupControllerGetCapabilities(ctx context.Context, req *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	fromType := func(
		cap csi.GroupControllerServiceCapability_RPC_Type,
	) *csi.GroupControllerServiceCapability {
		return &csi.GroupControllerServiceCapability{
			Type: &csi.GroupControllerServiceCapability_Rpc{
				Rpc: &csi.GroupControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	capabilities := make([]*csi.GroupControllerServiceCapability, 0)

	for _, cap := range []csi.GroupControllerServiceCapability_RPC_Type{
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
	} {
		capabilities = append(capabilities, fromType(cap))
	}
	return &csi.GroupControllerGetCapabilitiesResponse{Capabilities: capabilities}, nil
}


func (cs *controller) CreateVolumeGroupSnapshot(ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest) (*csi.CreateVolumeGroupSnapshotResponse, error) {
	vgSnapName := req.Name
	snapshots := make([]*csi.Snapshot, 0)
	snapTimeStamp := time.Now().Unix()
	volGroupSnapReady := true
	for _, volName := range req.SourceVolumeIds {

		// get vgName
		srcPV, err := cs.pvLister.Get(volName)
		if err != nil {
			log.Errorf("CreateSnapshot: fail to get pv: %s", err.Error())
			volGroupSnapReady = false
			continue
		}
		vgName := utils.GetVGNameFromCsiPV(srcPV)
		if vgName == "" {
			log.Errorf("CreateSnapshot: fail to get vgName of pv %s", srcPV.Name)
			volGroupSnapReady = false
			continue
		}
		snapshotName := fmt.Sprintf("%s-%s", vgSnapName, srcPV.Name)
		log.Infof("CreateSnapshot: snapshot %s in %s begin", snapshotName, vgName)

		// get nodeName
		nodeName := utils.GetNodeNameFromCsiPV(srcPV)
		if nodeName == "" {
			log.Errorf("CreateSnapshot: fail to get node name of pv %s", srcPV.Name)
			volGroupSnapReady = false
			continue
		}

		// get grpc client
		conn, err := cs.getNodeConn(nodeName)
		if err != nil {
			log.Errorf("CreateSnapshot: fail to get grpc client at node %s: %s", nodeName, err.Error())
			volGroupSnapReady = false
			continue
		}


		var sizeBytes int64

		// create lvm snapshot
		var volObj *remotelib.LogicalVolume
		if volObj, err = conn.GetVolume(ctx, vgName, snapshotName); err != nil {
			log.Errorf("CreateSnapshot: get lvm snapshot %s failed: %s", snapshotName, err.Error())
		} else {
			if volObj == nil {
				log.Infof("CreateSnapshot: ro snapshot %s not found, now creating on node %s", utils.GetNameKey(vgName, snapshotName), snapshotName, nodeName)
				sizeBytes, err = conn.CreateSnapshot(ctx, vgName, snapshotName, volName)
				if err != nil {
					log.Errorf("CreateSnapshot: create lvm snapshot %s failed: %s", snapshotName, err.Error())
					volGroupSnapReady = false
				} else {
					log.Infof("CreateSnapshot: create ro snapshot %s successfully", snapshotName)
				}

			} else {
				log.Infof("CreateSnapshot: lvm snapshot %s in node %s already exists", snapshotName, nodeName)
			}
		}

		if sizeBytes > 0 {

			snapshots = append(snapshots, csipayload.NewCreateSnapshotResponseBuilder().
				WithSourceVolumeID(volName).
				WithSnapshotID(snapshotName).
				WithCreationTime(snapTimeStamp, 0).
				WithReadyToUse(true).
				WithSize(sizeBytes).
				Build().Snapshot)
		}

		conn.Close()

	}

	return csipayload.NewCreateVolumeGroupSnapshotResponseBuilder().
		WithCreationTime(snapTimeStamp, 0).
		WithGroupSnapshotId(vgSnapName).WithSnapshots(snapshots).
		WithReadyToUse(volGroupSnapReady).Build(), nil
}

func (cs *controller) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (*csi.DeleteVolumeGroupSnapshotResponse, error) {
	klog.Infof("work with DeleteVolumeGroupSnapshot for %s", req.GroupSnapshotId)
	for _, snapshotID := range req.SnapshotIds {
		klog.Infof("work with DeleteSnapshot for %s", snapshotID)

		// get volumeID from snapshotcontent
		snapContent, err := utils.GetVolumeSnapshotContent(cs.snapClient, snapshotID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get snapContent %s error: %s", snapshotID, err.Error())
		}
		srcVolumeID := *snapContent.Spec.Source.VolumeHandle

		pv, err := cs.pvLister.Get(srcVolumeID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get pv: %s", err.Error())
		}
		vgName := utils.GetVGNameFromCsiPV(pv)
		if vgName == "" {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get vgName of pv %s", pv.Name)
		}
		log.Infof("DeleteSnapshot: vg of snapshot %s is %s", snapshotID, vgName)

		nodeName := utils.GetNodeNameFromCsiPV(pv)
		if nodeName == "" {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: fail to get node name of pv %s", pv.Name)
		}
		conn, err := cs.getNodeConn(nodeName)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get grpc client at node %s error: %s", nodeName, err.Error())
		}


		// delete lvm snapshot
		var volObj *remotelib.LogicalVolume
		if volObj, err = conn.GetVolume(ctx, vgName, snapshotID); err != nil {
			conn.Close()
			return nil, status.Errorf(codes.Internal, "DeleteSnapshot: get lvm snapshot %s failed: %s", snapshotID, err.Error())
		}
		if volObj != nil {
			log.Infof("DeleteSnapshot: lvm ro snapshot %s found, now deleting...", snapshotID)
			err := conn.DeleteSnapshot(ctx, vgName, snapshotID)
			if err != nil {
				conn.Close()
				return nil, status.Errorf(codes.Internal, "DeleteSnapshot: delete lvm snapshot %s failed: %s", snapshotID, err.Error())
			}
			log.Infof("DeleteSnapshot: lvm ro snapshot %s found, delete success", snapshotID)
		} else {
			log.Infof("DeleteSnapshot: lvm snapshot %s in node %s not found, skip...", snapshotID, nodeName)
		}
		conn.Close()
	}

	return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

func (cs *controller) GetVolumeGroupSnapshot(ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest) (*csi.GetVolumeGroupSnapshotResponse, error) {
	klog.Infof("work with GetVolumeGroupSnapshot for %s", req.GroupSnapshotId)
	return nil, status.Errorf(codes.Unimplemented, "method GetVolumeGroupSnapshot not implemented")
}

func getAllPVsOfLocalCSI() (PVsInfo map[string]corev1.PersistentVolume, err error) {
	allPVs := make(map[string]corev1.PersistentVolume)
	client, err := GetClientFromCacheOrCreate()
	if err != nil {
		return nil, err
	}

	pvs, err := client.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, pv := range pvs.Items {
		if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != config.LocalProvider {
			klog.V(5).Infof("csi source is nil or the volume is not managed by this local-csi")
			continue
		}
		if pv.Status.Phase != corev1.VolumeBound {
			klog.V(5).Infof("PV: %s status is not bound", pv.Name)
			continue
		}
		allPVs[pv.Name] = pv
	}

	return allPVs, nil
}

var ClientSet *kubernetes.Clientset

func GetClientFromCacheOrCreate() (*kubernetes.Clientset, error) {
	if ClientSet != nil {
		return ClientSet, nil
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		klog.Errorf("Failed to get k8s Incluster config. %+v", err)
		return nil, err
	}
	ClientSet, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("error building kubernetes clientset", err)
		return nil, err
	}

	return ClientSet, nil
}

func (cs *controller) validateDeleteVolumeReq(req *csi.DeleteVolumeRequest) error {
	volumeID := strings.ToLower(req.GetVolumeId())
	if volumeID == "" {
		return status.Error(
			codes.InvalidArgument,
			"failed to handle delete volume request: missing volume id",
		)
	}

	// volume should not be deleted if there are active snapshots present for the volume
	err := cs.validateRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"failed to handle delete volume request for {%s} : validation failed",
			volumeID,
		)
	}
	return nil
}

// IsSupportedVolumeCapabilityAccessMode valides the requested access mode
func IsSupportedVolumeCapabilityAccessMode(
	accessMode csi.VolumeCapability_AccessMode_Mode,
) bool {

	for _, access := range SupportedVolumeCapabilityAccessModes {
		if accessMode == access.Mode {
			return true
		}
	}
	return false
}

// newControllerCapabilities returns a list
// of this controller's capabilities
func newControllerCapabilities() []*csi.ControllerServiceCapability {
	fromType := func(
		cap csi.ControllerServiceCapability_RPC_Type,
	) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	var capabilities []*csi.ControllerServiceCapability
	for _, cap := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_GET_CAPACITY,
		csi.ControllerServiceCapability_RPC_VOLUME_CONDITION,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_GET_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	} {
		capabilities = append(capabilities, fromType(cap))
	}
	return capabilities
}

// validateRequest validates if the requested service is
// supported by the driver
func (cs *controller) validateRequest(
	c csi.ControllerServiceCapability_RPC_Type,
) error {

	for _, cap := range cs.capabilities {
		if c == cap.GetRpc().GetType() {
			return nil
		}
	}

	return status.Error(
		codes.InvalidArgument,
		fmt.Sprintf("failed to validate request: {%s} is not supported", c),
	)
}

func (cs *controller) validateVolumeCreateReq(req *csi.CreateVolumeRequest) error {
	err := cs.validateRequest(
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"failed to handle create volume request for {%s}",
			req.GetName(),
		)
	}

	if req.GetName() == "" {
		return status.Error(
			codes.InvalidArgument,
			"failed to handle create volume request: missing volume name",
		)
	}

	volCapabilities := req.GetVolumeCapabilities()
	if volCapabilities == nil {
		return status.Error(
			codes.InvalidArgument,
			"failed to handle create volume request: missing volume capabilities",
		)
	}

	validateSupportedVolumeCapabilities := func(volCap *csi.VolumeCapability) error {
		// VolumeCapabilities will contain volume mode
		if mode := volCap.GetAccessMode(); mode != nil {
			inputMode := mode.GetMode()
			// At the moment we only support SINGLE_NODE_WRITER i.e Read-Write-Once
			var isModeSupported bool
			for _, supporteVolCapability := range SupportedVolumeCapabilityAccessModes {
				if inputMode == supporteVolCapability.Mode {
					isModeSupported = true
					break
				}
			}

			if !isModeSupported {
				return status.Errorf(codes.InvalidArgument,
					"only ReadwriteOnce access mode is supported",
				)
			}
		}

		if volCap.GetBlock() == nil && volCap.GetMount() == nil {
			return status.Errorf(codes.InvalidArgument,
				"only Block mode (or) FileSystem mode is supported",
			)
		}

		return nil
	}

	for _, volCap := range volCapabilities {
		if err := validateSupportedVolumeCapabilities(volCap); err != nil {
			return err
		}
	}

	return nil
}

func (cs *controller) getNodeConn(nodeSelected string) (client.Connection, error) {
	node, err := cs.nodeLister.Get(nodeSelected)
	if err != nil {
		return nil, err
	}

	addr, err := getNodeAddr(node, nodeSelected, false)

	if err != nil {
		log.Errorf("CreateVolume: Get node %s address with error: %s", nodeSelected, err.Error())
		return nil, err
	}
	conn, err := client.NewGrpcConnection(addr, time.Duration(cs.driver.config.GrpcConnectionTimeout*int(time.Second)))
	return conn, err
}

func validateSnapshotRequest(req *csi.CreateSnapshotRequest) error {
	snapName := strings.ToLower(req.GetName())
	volumeID := strings.ToLower(req.GetSourceVolumeId())

	if snapName == "" || volumeID == "" {
		return status.Errorf(
			codes.InvalidArgument,
			"CreateSnapshot error invalid request %s: %s",
			volumeID, snapName,
		)
	}

	// TODO add capacity manager
	return nil
}
