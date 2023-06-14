/*
 Copyright © 2021 The OpenEBS Authors

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
	"strconv"
	"strings"


	"k8s.io/apimachinery/pkg/api/resource"
)

// VolumeParams holds collection of supported settings that can
// be configured in storage class.
type VolumeParams struct {
	PvcName string

	PvcNamespace string

	PvName string
	// HostPath specifies hostpath to use for
	// provisioning logical volumes.
	HostPath string
}

// SnapshotParams holds collection of supported settings that can
// be configured in snapshot class.
type SnapshotParams struct {
	SnapSize    float64
	AbsSnapSize bool
}

// NewVolumeParams parses the input params and instantiates new VolumeParams.
func NewVolumeParams(m map[string]string) (*VolumeParams, error) {
	params := &VolumeParams{ // set up defaults, if any.
	}
	// parameter keys may be mistyped from the CRD specification when declaring
	// the storageclass, which kubectl validation will not catch. Because
	// parameter keys (not values!) are all lowercase, keys may safely be forced
	// to the lower case.

	params.HostPath = m["hostpath"]

	// parse string params
	stringParams := map[string]*string{

	}
	for key, param := range stringParams {
		value, ok := m[key]
		if !ok {
			continue
		}
		*param = value
	}

	params.PvcName = m["csi.storage.k8s.io/pvc/name"]
	params.PvcNamespace = m["csi.storage.k8s.io/pvc/namespace"]
	params.PvName = m["csi.storage.k8s.io/pv/name"]
	return params, nil
}

// NewSnapshotParams parses the input params and instantiates new SnapshotParams.
func NewSnapshotParams(m map[string]string) (*SnapshotParams, error) {
	var err error
	params := &SnapshotParams{}

	// parameter keys may be mistyped from the CRD specification when declaring
	// the storageclass, which kubectl validation will not catch. Because
	// parameter keys (not values!) are all lowercase, keys may safely be forced
	// to the lower case.

	size, ok := m["snapsize"]
	if ok {
		if strings.HasSuffix(size, "%") {
			snapSize := size[:len(size)-1]
			params.SnapSize, err = strconv.ParseFloat(snapSize, 64)
			if err != nil {
				return nil, err
			}
			if params.SnapSize < 1 || params.SnapSize > 100 {
				return nil, fmt.Errorf("snapSize percentage should be between 1 and 100, found %s", size)
			}
		} else {
			qty, err := resource.ParseQuantity(size)
			if err != nil {
				return nil, err
			}
			snapSize, _ := qty.AsInt64()
			if snapSize < 1 {
				return nil, fmt.Errorf("absolute snapSize should greater than 0, found %s", size)
			}
			params.AbsSnapSize = true
			params.SnapSize = float64(snapSize)
		}
	}

	return params, nil
}
