package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	snapshotapi "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
)

const (
	VGName       = "vgName"

	EnvSnapshotPrefix     = "SNAPSHOT_PREFIX"
	DefaultSnapshotPrefix = "snap"
)

// CommandRunFunc define the run function in utils for ut
type CommandRunFunc func(cmd string) (string, error)

// Run run shell command
func Run(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Failed to run cmd: " + cmd + ", with out: " + string(out) + ", with error: " + err.Error())
	}
	return string(out), nil
}

func GetNameKey(nameSpace, name string) string {
	return fmt.Sprintf("%s/%s", nameSpace, name)
}

func GetVGNameFromCsiPV(pv *corev1.PersistentVolume) string {
	csi := pv.Spec.CSI
	if csi == nil {
		return ""
	}
	if v, ok := csi.VolumeAttributes[VGName]; ok {
		return v
	}
	log.V(6).Infof("PV %s has no csi volumeAttributes /%q", pv.Name, VGName)

	return ""
}


func GetNodeNameFromCsiPV(pv *corev1.PersistentVolume) string {
	if pv.Spec.NodeAffinity == nil {
		log.Errorf("pv %s with nil nodeAffinity", pv.Name)
		return ""
	}
	if pv.Spec.NodeAffinity.Required == nil || len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms) == 0 {
		log.Errorf("pv %s with nil Required or nil required.nodeSelectorTerms", pv.Name)
		return ""
	}
	if len(pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions) == 0 {
		log.Errorf("pv %s with nil MatchExpressions", pv.Name)
		return ""
	}
	key := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key
	if key != localtype.KubernetesNodeIdentityKey {
		log.Errorf("pv %s with MatchExpressions %s, must be %s", pv.Name, key, localtype.KubernetesNodeIdentityKey)
		return ""
	}

	nodes := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values
	if len(nodes) == 0 {
		log.Errorf("pv %s with empty nodes", pv.Name)
		return ""
	}
	nodeName := nodes[0]
	return nodeName
}

func GetVolumeSnapshotContent(snapclient snapshot.Interface, snapshotContentID string) (*snapshotapi.VolumeSnapshotContent, error) {
	// Step 1: get yoda snapshot prefix
	prefix := os.Getenv(EnvSnapshotPrefix)
	if prefix == "" {
		prefix = DefaultSnapshotPrefix
	}
	// Step 2: get snapshot content api
	return snapclient.SnapshotV1().VolumeSnapshotContents().Get(context.TODO(), strings.Replace(snapshotContentID, prefix, "snapcontent", 1), metav1.GetOptions{})
}



// IsBlockDevice checks if the given path is a block device
func IsBlockDevice(fullPath string) (bool, error) {
	var st unix.Stat_t
	err := unix.Stat(fullPath, &st)
	if err != nil {
		return false, err
	}

	return (st.Mode & unix.S_IFMT) == unix.S_IFBLK, nil
}

func EnsureBlock(target string) error {
	fi, err := os.Lstat(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && fi.IsDir() {
		os.Remove(target)
	}
	targetPathFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, 0750)
	if err != nil {
		return fmt.Errorf("failed to create block:%s with error: %v", target, err)
	}
	if err := targetPathFile.Close(); err != nil {
		return fmt.Errorf("failed to close targetPath:%s with error: %v", target, err)
	}
	return nil
}


func MountBlock(source, target string, opts ...string) error {
	mountCmd := "mount"
	mountArgs := []string{}

	if source == "" {
		return errors.New("source is not specified for mounting the volume")
	}
	if target == "" {
		return errors.New("target is not specified for mounting the volume")
	}

	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}
	mountArgs = append(mountArgs, source)
	mountArgs = append(mountArgs, target)
	// create target, os.Mkdirall is noop if it exists
	_, err := os.Create(target)
	if err != nil {
		return err
	}

	log.V(6).Infof("Mount %s to %s, the command is %s %v", source, target, mountCmd, mountArgs)
	out, err := exec.Command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}
	return nil
}

