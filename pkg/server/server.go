/*
Copyright 2020 The Kubernetes Authors.

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

package server

import (
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"os"

	"github.com/google/credstore/client"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/opentracing/opentracing-go"
	"github.com/winrouter/csi-hostpath/pkg/lib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	log "k8s.io/klog/v2"
)

const (
	lvmdPort = "9091"
)

// Start start lvmd
func Start(port string) {
	var cmd VolCmd
	cmd = nil

	if cmd == nil {
		log.Errorf("retrieve nls failed, try to run as LVM")
		cmd = &HostPathCommads{}
	}

	svr := NewServer(cmd)

	address := fmt.Sprintf(":%s", port)
	log.Infof("Lvmd Starting with socket: %s ...", address)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer, _, err := NewGRPCServer()
	if err != nil {
		log.Errorf("failed to init GRPC server: %v", err)
		return
	}

	lib.RegisterVolServer(grpcServer, &svr)

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
	log.Infof("Lvmd End ...")
}



// NewServer creates a new GRPC server stub with credstore auth (if requested).
func NewGRPCServer() (*grpc.Server, *client.CredstoreClient, error) {
	var grpcServer *grpc.Server
	var cc *client.CredstoreClient

	grpcServer = grpc.NewServer(
		grpc.UnaryInterceptor(
			otgrpc.OpenTracingServerInterceptor(opentracing.GlobalTracer())))

	reflection.Register(grpcServer)
	grpcprometheus.Register(grpcServer)

	return grpcServer, cc, nil
}

// GetLvmdPort get lvmd port
func GetLvmdPort() string {
	return lvmdPort
}

func newCmd() (VolCmd, error) {
	nodeName := os.Getenv("KUBE_NODE_NAME")
	log.Infof("new Cmd with NodeName %s", nodeName)
	return &HostPathCommads{}, nil
}

type VolCmd interface {
	ListVol(listspec string) ([]*lib.Vol, error)
	CreateVol(ctx context.Context, vg string, name string, size uint64) (string, error)
	RemoveVol(ctx context.Context, vg string, name string) (string, error)
	CloneVol(ctx context.Context, vgName, src, dest string) (string, error)
	ExpandVol(ctx context.Context, vgName string, volumeId string, expectSize uint64) (string, error)
	CreateSnapshot(ctx context.Context, vgName string, snapshotName string, srcVolumeName string) (int64, error)
	RemoveSnapshot(ctx context.Context, vg string, name string) (string, error)
}


// Server lvm grpc server
type Server struct {
	lib.UnimplementedVolServer
	impl VolCmd
}

// NewServer new server
func NewServer(cmd VolCmd) Server {
	return Server{impl: cmd}
}

// ListVol list lvm volume
func (s Server) ListVol(ctx context.Context, in *lib.ListVolRequest) (*lib.ListVolReply, error) {
	log.V(6).Infof("List LVM for vg: %s", in.VolumeGroup)
	lvs, err := s.impl.ListVol(in.VolumeGroup)
	if err != nil {
		log.Errorf("List LVM with error: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to list LVs: %v", err)
	}

	pblvs := make([]*lib.LogicalVolume, len(lvs))
	for i, v := range lvs {
		pblvs[i] = v.ToProto()
	}
	log.V(6).Infof("List LVM Successful with result: %+v", pblvs)
	return &lib.ListVolReply{Volumes: pblvs}, nil
}

// CreateLV create lvm volume
func (s Server) CreateVol(ctx context.Context, in *lib.CreateVolRequest) (*lib.CreateVolReply, error) {
	log.V(6).Infof("Create LVM with: %+v", in)
	out, err := s.impl.CreateVol(ctx, in.VolumeGroup, in.Name, in.Size)
	if err != nil {
		log.Errorf("Create LVM with error: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to create lv: %v", err)
	}
	log.V(6).Infof("Create LVM Successful with result: %+v", out)
	return &lib.CreateVolReply{CommandOutput: out}, nil
}

// RemoveLV remove lvm volume
func (s Server) RemoveVol(ctx context.Context, in *lib.RemoveVolRequest) (*lib.RemoveVolReply, error) {
	log.V(6).Infof("Remove LVM with: %+v", in)
	out, err := s.impl.RemoveVol(ctx, in.VolumeGroup, in.Name)
	if err != nil {
		log.Errorf("Remove LVM with error: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to remove lv: %v", err)
	}
	log.V(6).Infof("Remove LVM Successful with result: %+v", out)
	return &lib.RemoveVolReply{CommandOutput: out}, nil
}

// CloneLV clone lvm volume
func (s Server) CloneVol(ctx context.Context, in *lib.CloneVolRequest) (*lib.CloneVolReply, error) {
	out, err := s.impl.CloneVol(ctx, in.VgName, in.SourceName, in.DestName)
	if err != nil {
		log.Errorf("Clone LVM with error: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to clone lv: %v", err)
	}
	log.V(6).Infof("Clone LVM with result: %+v", out)
	return &lib.CloneVolReply{CommandOutput: out}, nil
}

// ExpandLV expand lvm volume
func (s Server) ExpandVol(ctx context.Context, in *lib.ExpandVolRequest) (*lib.ExpandVolReply, error) {
	out, err := s.impl.ExpandVol(ctx, in.VolumeGroup, in.Name, in.Size)
	if err != nil {
		log.Errorf("Expand LVM with error: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to expand lv: %v", err)
	}
	log.V(6).Infof("Expand LVM with result: %+v", out)
	return &lib.ExpandVolReply{CommandOutput: out}, nil
}


// CreateSnapshot create lvm snapshot
func (s Server) CreateSnapshot(ctx context.Context, in *lib.CreateSnapshotRequest) (*lib.CreateSnapshotReply, error) {
	log.V(6).Infof("create snapshot with: %+v", in)
	sizeBytes, err := s.impl.CreateSnapshot(ctx, in.VgName, in.SnapshotName, in.SrcVolumeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fail to create snapshot %s: %s", in.SnapshotName, err.Error())
	}
	log.V(6).Infof("create snapshot successfully with result: %+v", sizeBytes)
	return &lib.CreateSnapshotReply{SizeBytes: sizeBytes}, nil
}

// RemoveSnapshot remove lvm snapshot
func (s Server) RemoveSnapshot(ctx context.Context, in *lib.RemoveSnapshotRequest) (*lib.RemoveSnapshotReply, error) {
	log.V(6).Infof("Remove LVM Snapshot with: %+v", in)
	out, err := s.impl.RemoveSnapshot(ctx, in.VgName, in.SnapshotName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "RemoveSnapshot: remove snapshot with error: %s", err.Error())
	}
	log.V(6).Infof("Remove LVM Snapshot Successful with result: %+v", out)
	return &lib.RemoveSnapshotReply{CommandOutput: out}, nil
}


