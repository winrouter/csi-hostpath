package driver

import (
	"fmt"
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	remotelib "github.com/winrouter/csi-hostpath/pkg/lib"
	csipayload "github.com/winrouter/csi-hostpath/pkg/response"
	"github.com/winrouter/csi-hostpath/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	log "k8s.io/klog/v2"
	"time"
)

func newGroupControllerCapabilities() []*csi.GroupControllerServiceCapability {
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
	return capabilities
}

func (cs *controller) GroupControllerGetCapabilities(ctx context.Context, req *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {

	return &csi.GroupControllerGetCapabilitiesResponse{Capabilities: cs.groupCapabilities}, nil
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
				WithGroupSnapshotId(vgSnapName).
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