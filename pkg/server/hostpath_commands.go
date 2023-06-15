/*

Copyright 2017 Google Inc.

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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/winrouter/csi-hostpath/pkg/lib"
	"golang.org/x/net/context"
	log "k8s.io/klog/v2"
)

type HostPathCommads struct {
}

func getVolMeta(volgroup, vol string) (*lib.Vol, error) {
	// whether meta.json of vol exist
	metaFile := fmt.Sprintf("/opt/csi-hostpath/%s/%s/meta.json", volgroup, vol)
	f, err := os.Open(metaFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	volObj := lib.Vol{}
	// meta.json
	// {name: volName, size: 123456}
	err = json.Unmarshal([]byte(out), &volObj)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal out:%s err:%+v", out, err)
	}
	return &volObj, nil
}

// ListLV lists lvm volumes
func (lvm *HostPathCommads) ListVol(volgroup string) ([]*lib.Vol, error) {
	log.Infof("exec ListVol with paras: %s", volgroup)
	vols := []*lib.Vol{}

	dirPath := fmt.Sprintf("/opt/csi-hostpath/%s", volgroup)
	f, err := os.Open(dirPath)
	if err != nil {
		log.Errorf("failed to read dir %s", volgroup)
		return vols, nil
	}

	dirs, err := f.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("failed to walkup dir %s", volgroup)
	}

	for _, dirEntry := range dirs {
		if !dirEntry.IsDir() {
			continue
		}

		vol, err := getVolMeta(volgroup, dirEntry.Name())
		if err != nil {
			return nil, errors.New("Parse metadata: " + dirEntry.Name() + ", with error: " + err.Error())
		}
		vols = append(vols, vol)
	}

	return vols, nil
}

// CreateLV creates a new volume
func (lvm *HostPathCommads) CreateVol(ctx context.Context, vg string, name string, size uint64) (string, error) {
	volPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s", vg, name)

	err := os.MkdirAll(volPath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create %s, err:%v", volPath, err)
	}

	mountPoint := fmt.Sprintf("%s/mountpoint", volPath)
	err = os.MkdirAll(mountPoint, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to mount point %s, err:%v", volPath, err)
	}

	vol := lib.Vol {
		Name: name,
		Size: size,
		VGName: vg,
		Status: "ok",
	}

	metaFile := fmt.Sprintf("%s/meta.json", volPath)
	f, err := os.OpenFile(metaFile, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata %s, err:%v", metaFile, err)
	}

	volBytes, err := json.Marshal(vol)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to metadata %s, err:%v", metaFile, err)
	}

	_, err = io.WriteString(f, string(volBytes))
	if err != nil {
		return "", fmt.Errorf("failed to write metadata to %s, err:%v", metaFile, err)
	}

	return name, nil
}

// RemoveLV removes a volume
func (lvm *HostPathCommads) RemoveVol(ctx context.Context, vg string, name string) (string, error) {

	// TODO: check if lv has snapshot
	volPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s", vg, name)
	os.RemoveAll(volPath)
	return name, nil
}

// CloneLV clones a volume via dd
func (lvm *HostPathCommads) CloneVol(ctx context.Context, vg, src, dest string) (string, error) {
	// FIXME(farcaller): bloody insecure. And broken.

	// step1: get vol metadata
	srcVolInfo, err := getVolMeta(vg, src)
	if err != nil {
		return "", fmt.Errorf("failed to load vol %s metadata, err: %v", src, err)
	}

	destVolInfo := srcVolInfo
	destVolInfo.Name = dest

	srcPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s", vg, src)
	destPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s", vg, dest)

	err = os.MkdirAll(destPath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create %s, err:%v", destPath, err)
	}

	// step2: copy data to dest mountpoint
	destMountPoint := fmt.Sprintf("%s/mountpoint", destPath)
	err = os.MkdirAll(destMountPoint, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to mount point %s, err:%v", destPath, err)
	}

	srcMountPoint := fmt.Sprintf("%s/mountpoint", srcPath)

	err = filepath.Walk(srcMountPoint, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			return err
		}

		if path == srcMountPoint {
			return nil
		}

		destNewPath := strings.Replace(path, srcMountPoint, destMountPoint, -1)
		if !f.IsDir() {
			CopyFile(path, destNewPath)
		} else {
			if !FileIsExisted(destNewPath) {
				return MakeDir(destNewPath)
			}
		}
		return nil
	})

	// step3: write dest meta json
	metaFile := fmt.Sprintf("%s/meta.json", destPath)
	f, err := os.OpenFile(metaFile, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata %s, err:%v", metaFile, err)
	}

	volBytes, err := json.Marshal(destVolInfo)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to metadata %s, err:%v", metaFile, err)
	}

	_, err = io.WriteString(f, string(volBytes))
	if err != nil {
		return "", fmt.Errorf("failed to write metadata to %s, err:%v", metaFile, err)
	}

	return dest, nil
}

// ExpandLV expand a volume
func (lvm *HostPathCommads) ExpandVol(ctx context.Context, vg string, vol string, expectSize uint64) (string, error) {
	// step1: get vol metadata
	volInfo, err := getVolMeta(vg, vol)
	if err != nil {
		return "", fmt.Errorf("failed to load vol %s metadata, err: %v", vol, err)
	}

	volInfo.Size = expectSize

	metaFile := fmt.Sprintf("/opt/csi-hostpath/%s/%s/meta.json", vg, vol)
	os.Remove(metaFile)
	f, err := os.OpenFile(metaFile, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata %s, err:%v", metaFile, err)
	}

	volBytes, err := json.Marshal(volInfo)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to metadata %s, err:%v", metaFile, err)
	}

	_, err = io.WriteString(f, string(volBytes))
	if err != nil {
		return "", fmt.Errorf("failed to write metadata to %s, err:%v", metaFile, err)
	}

	return vol, nil
}


// CreateSnapshot creates a new volume snapshot
func (lvm *HostPathCommads) CreateSnapshot(ctx context.Context, vg string, snapshot string, src string) (int64, error) {
	// step1: get vol metadata
	srcVolInfo, err := getVolMeta(vg, src)
	if err != nil {
		return 0, fmt.Errorf("failed to load vol %s metadata, err: %v", src, err)
	}

	snapVolInfo := srcVolInfo
	snapVolInfo.Name = snapshot

	srcPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s", vg, src)
	snapPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s", vg, snapshot)

	err = os.MkdirAll(snapPath, os.ModePerm)
	if err != nil {
		return 0, fmt.Errorf("failed to create %s, err:%v", snapPath, err)
	}

	// step2: copy data to dest mountpoint
	snapMountPoint := fmt.Sprintf("%s/mountpoint", snapPath)
	err = os.MkdirAll(snapMountPoint, os.ModePerm)
	if err != nil {
		return 0, fmt.Errorf("failed to mount point %s, err:%v", snapPath, err)
	}

	srcMountPoint := fmt.Sprintf("%s/mountpoint", srcPath)

	err = filepath.Walk(srcMountPoint, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			return err
		}

		if path == srcMountPoint {
			return nil
		}

		snapNewPath := strings.Replace(path, srcMountPoint, snapMountPoint, -1)
		if !f.IsDir() {
			CopyFile(path, snapNewPath)
		} else {
			if !FileIsExisted(snapNewPath) {
				return MakeDir(snapNewPath)
			}
		}
		return nil
	})

	// step3: write dest meta json
	metaFile := fmt.Sprintf("%s/meta.json", snapPath)
	f, err := os.OpenFile(metaFile, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return 0, fmt.Errorf("failed to create metadata %s, err:%v", metaFile, err)
	}

	volBytes, err := json.Marshal(snapVolInfo)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal to metadata %s, err:%v", metaFile, err)
	}

	_, err = io.WriteString(f, string(volBytes))
	if err != nil {
		return 0, fmt.Errorf("failed to write metadata to %s, err:%v", metaFile, err)
	}

	return 0, nil
}

func (lvm *HostPathCommads) GetVolPath(ctx context.Context, vg, vol string) string {
	if vg == "" || vol == "" {
		return ""
	}
	volPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s/mountpoint", vg, vol)
	return volPath
}

// RemoveSnapshot removes a volume snapshot
func (lvm *HostPathCommads) RemoveSnapshot(ctx context.Context, vg string, name string) (string, error) {
	volPath := fmt.Sprintf("/opt/csi-hostpath/%s/%s", vg, name)
	os.RemoveAll(volPath)
	return "", nil
}

func FileIsExisted(filename string) bool {
	existed := true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		existed = false
	}
	return existed
}

func CopyFile(src, des string) (written int64, err error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer srcFile.Close()

	//获取源文件的权限
	fi, _ := srcFile.Stat()
	perm := fi.Mode()

	//desFile, err := os.Create(des)  //无法复制源文件的所有权限
	desFile, err := os.OpenFile(des, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)  //复制源文件的所有权限
	if err != nil {
		return 0, err
	}
	defer desFile.Close()

	return io.Copy(desFile, srcFile)
}

func MakeDir(dir string) error {
	if !FileIsExisted(dir) {
		if err := os.MkdirAll(dir, 0777); err != nil { //os.ModePerm
			fmt.Println("MakeDir failed:", err)
			return err
		}
	}
	return nil
}

