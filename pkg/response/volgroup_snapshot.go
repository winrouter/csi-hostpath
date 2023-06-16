package v1alpha1


import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
)

// CreateSnapshotResponseBuilder helps building an
// instance of csi CreateVolumeResponse
type CreateVolumeGroupSnapshotResponseBuilder struct {
	response *csi.CreateVolumeGroupSnapshotResponse
}

// NewCreateSnapshotResponseBuilder returns a new
// instance of CreateSnapshotResponseBuilder
func NewCreateVolumeGroupSnapshotResponseBuilder() *CreateVolumeGroupSnapshotResponseBuilder {
	return &CreateVolumeGroupSnapshotResponseBuilder{
		response: &csi.CreateVolumeGroupSnapshotResponse{
			GroupSnapshot: &csi.VolumeGroupSnapshot{},
		},
	}
}

// WithSize sets the size against the
// CreateSnapshotResponse instance
func (b *CreateVolumeGroupSnapshotResponseBuilder) WithGroupSnapshotId(groupSnapshotId string) *CreateVolumeGroupSnapshotResponseBuilder {
	b.response.GroupSnapshot.GroupSnapshotId = groupSnapshotId
	return b
}

// WithSnapshotID sets the snapshotID against the
// CreateSnapshotResponse instance
func (b *CreateVolumeGroupSnapshotResponseBuilder) WithSnapshots(snapshots []*csi.Snapshot) *CreateVolumeGroupSnapshotResponseBuilder {
	b.response.GroupSnapshot.Snapshots = snapshots
	return b
}



// WithCreationTime sets the creationTime against the
// CreateSnapshotResponse instance
func (b *CreateVolumeGroupSnapshotResponseBuilder) WithCreationTime(tsec, tnsec int64) *CreateVolumeGroupSnapshotResponseBuilder {
	b.response.GroupSnapshot.CreationTime = &timestamp.Timestamp{
		Seconds: tsec,
		Nanos:   int32(tnsec),
	}
	return b
}

// WithReadyToUse sets the readyToUse field against the
// CreateSnapshotResponse instance
func (b *CreateVolumeGroupSnapshotResponseBuilder) WithReadyToUse(readyToUse bool) *CreateVolumeGroupSnapshotResponseBuilder {
	b.response.GroupSnapshot.ReadyToUse = readyToUse
	return b
}

// Build returns the constructed instance
// of csi CreateSnapshotResponse
func (b *CreateVolumeGroupSnapshotResponseBuilder) Build() *csi.CreateVolumeGroupSnapshotResponse {
	return b.response
}
