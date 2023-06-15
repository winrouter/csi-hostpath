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

package lib



// VolumeType is volume type
type VolumeType byte

var volumeTypeKeys = []byte("mMoOrRsSpviIlcVtTe")

// types
const (
	VolumeTypeMirrored                  VolumeType = 'm'
	VolumeTypeMirroredWithoutSync       VolumeType = 'M'
	VolumeTypeOrigin                    VolumeType = 'o'
	VolumeTypeOriginWithMergingSnapshot VolumeType = 'O'
	VolumeTypeRAID                      VolumeType = 'r'
	VolumeTypeRAIDWithoutSync           VolumeType = 'R'
	VolumeTypeSnapshot                  VolumeType = 's'
	VolumeTypeMergingSnapshot           VolumeType = 'S'
	VolumeTypePVMove                    VolumeType = 'p'
	VolumeTypeVirtualMirror             VolumeType = 'v'
	VolumeTypeVirtualRaidImage          VolumeType = 'i'
	VolumeTypeRaidImageOutOfSync        VolumeType = 'I'
	VolumeTypeMirrorLog                 VolumeType = 'l'
	VolumeTypeUnderConversion           VolumeType = 'c'
	VolumeTypeThin                      VolumeType = 'V'
	VolumeTypeThinPool                  VolumeType = 't'
	VolumeTypeThinPoolData              VolumeType = 'T'
	VolumeTypeRaidOrThinPoolMetadata    VolumeType = 'e'
)



// VolumePermissions is volume permissions
type VolumePermissions rune

var volumePermissonsKeys = []byte("wrR")

// permissions
const (
	VolumePermissionsWriteable          VolumePermissions = 'w'
	VolumePermissionsReadOnly           VolumePermissions = 'r'
	VolumePermissionsReadOnlyActivation VolumePermissions = 'R'
)


// VolumeAllocation is volume allocation policy
type VolumeAllocation rune

var volumeAllocationKeys = []byte("acilnACILN")

// allocations
const (
	VolumeAllocationAnywhere         VolumeAllocation = 'a'
	VolumeAllocationContiguous       VolumeAllocation = 'c'
	VolumeAllocationInherited        VolumeAllocation = 'i'
	VolumeAllocationCling            VolumeAllocation = 'l'
	VolumeAllocationNormal           VolumeAllocation = 'n'
	VolumeAllocationAnywhereLocked   VolumeAllocation = 'A'
	VolumeAllocationContiguousLocked VolumeAllocation = 'C'
	VolumeAllocationInheritedLocked  VolumeAllocation = 'I'
	VolumeAllocationClingLocked      VolumeAllocation = 'L'
	VolumeAllocationNormalLocked     VolumeAllocation = 'N'
)



// VolumeFixedMinor is volume fixed minor
type VolumeFixedMinor rune

// fixed minor
const (
	VolumeFixedMinorEnabled  VolumeFixedMinor = 'm'
	VolumeFixedMinorDisabled VolumeFixedMinor = '-'
)

func (t VolumeFixedMinor) toProto() bool {
	return t == VolumeFixedMinorEnabled
}

// VolumeState is volume state
type VolumeState rune

var volumeStateKeys = []byte("asISmMdi")

// states
const (
	VolumeStateActive                               VolumeState = 'a'
	VolumeStateSuspended                            VolumeState = 's'
	VolumeStateInvalidSnapshot                      VolumeState = 'I'
	VolumeStateInvalidSuspendedSnapshot             VolumeState = 'S'
	VolumeStateSnapshotMergeFailed                  VolumeState = 'm'
	VolumeStateSuspendedSnapshotMergeFailed         VolumeState = 'M'
	VolumeStateMappedDevicePresentWithoutTables     VolumeState = 'd'
	VolumeStateMappedDevicePresentWithInactiveTable VolumeState = 'i'
)



// VolumeOpen is volume open
type VolumeOpen rune

// open
const (
	VolumeOpenIsOpen    VolumeOpen = 'o'
	VolumeOpenIsNotOpen VolumeOpen = '-'
)

func (t VolumeOpen) toProto() bool {
	return t == VolumeOpenIsOpen
}

// VolumeTargetType is volume taget type
type VolumeTargetType rune

var volumeTargetTypeKeys = []byte("mrstuv")

// target type
const (
	VolumeTargetTypeMirror   VolumeTargetType = 'm'
	VolumeTargetTypeRAID     VolumeTargetType = 'r'
	VolumeTargetTypeSnapshot VolumeTargetType = 's'
	VolumeTargetTypeThin     VolumeTargetType = 't'
	VolumeTargetTypeUnknown  VolumeTargetType = 'u'
	VolumeTargetTypeVirtual  VolumeTargetType = 'v'
)



// VolumeZeroing is volume zeroing
type VolumeZeroing rune

// zeroing
const (
	VolumeZeroingIsZeroing    VolumeZeroing = 'z'
	VolumeZeroingIsNonZeroing VolumeZeroing = '-'
)

func (t VolumeZeroing) toProto() bool {
	return t == VolumeZeroingIsZeroing
}

// VolumeHealth is volume health
type VolumeHealth rune

// health
const (
	VolumeHealthOK              VolumeHealth = '-'
	VolumeHealthPartial         VolumeHealth = 'p'
	VolumeHealthRefreshNeeded   VolumeHealth = 'r'
	VolumeHealthMismatchesExist VolumeHealth = 'm'
	VolumeHealthWritemostly     VolumeHealth = 'w'
)



// VolumeActivationSkipped is volume activation
type VolumeActivationSkipped rune

// activation
const (
	VolumeActivationSkippedIsSkipped    VolumeActivationSkipped = 's'
	VolumeActivationSkippedIsNotSkipped VolumeActivationSkipped = '-'
)

func (t VolumeActivationSkipped) toProto() bool {
	return t == VolumeActivationSkippedIsSkipped
}

// LVAttributes is attributes
type LVAttributes struct {
	Type              VolumeType
	Permissions       VolumePermissions
	Allocation        VolumeAllocation
	FixedMinor        VolumeFixedMinor
	State             VolumeState
	Open              VolumeOpen
	TargetType        VolumeTargetType
	Zeroing           VolumeZeroing
	Health            VolumeHealth
	ActivationSkipped VolumeActivationSkipped
}



// LV is a logical volume
type Vol struct {
	Name               string
	Size               uint64
	Tags               []string
	VGName             string
	Status             string
	SnapshotName       string
	CloneName          string
}

// VG is volume group
type VG struct {
	Name     string
	Size     uint64
	FreeSize uint64
	UUID     string
	Tags     []string
	PvCount  uint64
}

// PV is physical volume
type PV struct {
	Name     string
	Size     uint64
	FreeSize uint64
	UUID     string
	Tags     []string
	VgName   string
}

// ToProto returns lvm.LogicalVolume representation of struct
func (lv Vol) ToProto() *LogicalVolume {
	return &LogicalVolume{
		Name:                 lv.Name,
		Size:                 lv.Size,
		VolumeGroup:  lv.VGName,
		Status: lv.Status,
		Tags:                 lv.Tags,
		SnapshotName: lv.SnapshotName,
		CloneName: lv.CloneName,
	}
}

// ToProto to proto
func (vg VG) ToProto() *VolumeGroup {
	return &VolumeGroup{
		Name:     vg.Name,
		Size:     vg.Size,
		FreeSize: vg.FreeSize,
		Uuid:     vg.UUID,
		Tags:     vg.Tags,
	}
}
