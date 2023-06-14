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

package client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	remotelib "github.com/winrouter/csi-hostpath/pkg/lib"

	"github.com/winrouter/csi-hostpath/pkg/utils"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	log "k8s.io/klog/v2"
)

// Connection lvm connection interface
type Connection interface {
	GetVolume(ctx context.Context, volGroup string, volumeID string) (string, error)
	CreateVolume(ctx context.Context, opt *VolOptions) (string, error)
	DeleteVolume(ctx context.Context, volGroup string, volumeID string) error
	CreateSnapshot(ctx context.Context, vgName string, snapshotName string, srcVolumeName string) (int64, error)
	DeleteSnapshot(ctx context.Context, volGroup string, snapVolumeID string) error
	ExpandVolume(ctx context.Context, volGroup string, volumeID string, size uint64) error
	Close() error
}

// VolOptions lvm options
type VolOptions struct {
	VolumeGroup string   `json:"volumeGroup,omitempty"`
	Name        string   `json:"name,omitempty"`
	Size        uint64   `json:"size,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	CloneName    string     `json:"cloneName,omitempty"`
	SnapshotName    string     `json:"snapshotName,omitempty"`
}

type workerConnection struct {
	conn *grpc.ClientConn
}

var (
	_ Connection = &workerConnection{}
)



// NewGrpcConnection lvm connection
func NewGrpcConnection(address string, timeout time.Duration) (Connection, error) {
	conn, err := connect(address, timeout)
	if err != nil {
		return nil, err
	}
	return &workerConnection{
		conn: conn,
	}, nil
}

func (c *workerConnection) Close() error {
	return c.conn.Close()
}

func connect(address string, timeout time.Duration) (*grpc.ClientConn, error) {
	log.V(6).Infof("New Connecting to %s", address)
	// only for unit test
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(logGRPC),
	}

	// if strings.HasPrefix(address, "/") {
	// 	dialOptions = append(dialOptions, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
	// 		return net.DialTimeout("unix", addr, timeout)
	// 	}))
	// }
	if strings.HasPrefix(address, "/") {
		dialOptions = append(
			dialOptions,
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", addr, timeout)
			}))
	}
	conn, err := grpc.Dial(address, dialOptions...)

	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		if !conn.WaitForStateChange(ctx, conn.GetState()) {
			log.Warningf("Connection to %s timed out", address)
			return conn, nil // return nil, subsequent GetPluginInfo will show the real connection error
		}
		if conn.GetState() == connectivity.Ready {
			log.V(6).Infof("Connected to %s", address)
			return conn, nil
		}
		log.V(6).Infof("Still trying to connect %s, connection is %s", address, conn.GetState())
	}
}

func (c *workerConnection) CreateVolume(ctx context.Context, opt *VolOptions) (string, error) {
	client := remotelib.NewVolClient(c.conn)
	req := remotelib.CreateVolRequest{
		VolumeGroup: opt.VolumeGroup,
		Name:        opt.Name,
		Size:        opt.Size,
		Tags:        opt.Tags,
		CloneName:   opt.CloneName,
		SnapshotName:opt.SnapshotName,
	}

	rsp, err := client.CreateVol(ctx, &req)
	if err != nil {
		log.Errorf("Create Lvm with error: %s", err.Error())
		return "", err
	}
	log.V(6).Infof("Create Lvm with result: %+v", rsp.CommandOutput)
	return rsp.GetCommandOutput(), nil
}

func (c *workerConnection) CreateSnapshot(ctx context.Context, vgName string, snapshotName string, srcVolumeName string) (int64, error) {
	client := remotelib.NewVolClient(c.conn)

	req := remotelib.CreateSnapshotRequest{
		VgName:        vgName,
		SnapshotName:  snapshotName,
		SrcVolumeName: srcVolumeName,
	}
	rsp, err := client.CreateSnapshot(ctx, &req)
	if err != nil {
		return 0, fmt.Errorf("fail to create snapshot: %s", err.Error())
	}
	return rsp.GetSizeBytes(), nil
}

func (c *workerConnection) GetVolume(ctx context.Context, volGroup string, volumeID string) (string, error) {
	client := remotelib.NewVolClient(c.conn)
	req := remotelib.ListVolRequest{
		VolumeGroup: volGroup,
	}

	rsp, err := client.ListVol(ctx, &req)
	if err != nil {
		log.Errorf("Get Lvm with error: %s", err.Error())
		return "", err
	}
	log.V(6).Infof("Get Lvm with result: %+v", rsp.Volumes)

	for _, volume := range rsp.GetVolumes() {
		if volume.Name == volumeID {
			return volumeID, nil
		}
	}

	log.Warningf("Volume %s is not exist", utils.GetNameKey(volGroup, volumeID))
	return "", nil
}

func (c *workerConnection) DeleteVolume(ctx context.Context, volGroup, volumeID string) error {
	client := remotelib.NewVolClient(c.conn)
	req := remotelib.RemoveVolRequest{
		VolumeGroup: volGroup,
		Name:        volumeID,
	}
	response, err := client.RemoveVol(ctx, &req)
	if err != nil {
		log.Errorf("Remove Lvm with error: %v", err.Error())
		return err
	}
	log.V(6).Infof("Remove Lvm with result: %v", response.GetCommandOutput())
	return err
}

func (c *workerConnection) DeleteSnapshot(ctx context.Context, volGroup string, snapVolumeID string) error {
	client := remotelib.NewVolClient(c.conn)
	req := remotelib.RemoveSnapshotRequest{
		VgName:       volGroup,
		SnapshotName: snapVolumeID,

	}
	response, err := client.RemoveSnapshot(ctx, &req)
	if err != nil {
		log.Errorf("Remove Lvm Snapshot with error: %v", err.Error())
		return err
	}
	log.V(6).Infof("Remove Lvm Snapshot with result: %v", response.GetCommandOutput())
	return err
}


func (c *workerConnection) ExpandVolume(ctx context.Context, volGroup string, volumeID string, size uint64) error {
	client := remotelib.NewVolClient(c.conn)
	req := remotelib.ExpandVolRequest{
		VolumeGroup: volGroup,
		Name:        volumeID,
		Size:        size,
	}
	response, err := client.ExpandVol(ctx, &req)
	if err != nil {
		log.Errorf("Expand Lvm with error: %v", err.Error())
		return err
	}
	log.V(6).Infof("Expand Lvm with result: %v", response.GetCommandOutput())
	return err
}

func logGRPC(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	log.V(6).Infof("GRPC request: %s, %+v", method, req)
	err := invoker(ctx, method, req, reply, cc, opts...)
	log.V(6).Infof("GRPC response: %+v, %v", reply, err)
	return err
}