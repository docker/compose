package containerkit

import "errors"

var (
	errNotImplemented = errors.New("not implemented")
)

// LayerManipulator provides an API for allocating, snapshotting and mounting
// abstract, layer-based filesytems. The model works by building up sets of
// directories with parent-child relationships.
//
// These differ from the concept of the graphdriver in that the
// LayerManipulator has no knowledge of images or containers. Users simply
// prepare and commit directories. We also avoid the integration between graph
// driver's and the tar format used to represent the changesets.
//
// Importing a Layer
//
// To import a layer, we simply have the LayerManipulator provide a list of
// mounts to be applied such that our dst will capture a changeset. We start
// out by getting a path to the layer tar file and creating a temp location to
// unpack it to:
//
//	layerPath, tmpLocation := getLayerPath(), mkTmpDir() // just a path to layer tar file.
//
// We then use a LayerManipulator to prepare the temporary location as a
// snapshot point:
//
// 	lm := NewLayerManipulator()
//	mounts, err := lm.Prepare(tmpLocation, "")
// 	if err != nil { ... }
//
// Note that we provide "" as the parent, since we are applying the diff to an
// empty directory. We get back a list of mounts from LayerManipulator.Prepare.
// Before proceeding, we perform all these mounts:
//
//	if err := MountAll(mounts); err != nil { ... }
//
// Once the mounts are performed, our temporary location is ready to capture
// a diff. In practice, this works similar to a filesystem transaction. The
// next step is to unpack the layer. We have a special function unpackLayer
// that applies the contents of the layer to target location and calculates the
// DiffID of the unpacked layer (this is a requirement for docker
// implementation):
//
// 	digest, err := unpackLayer(tmpLocation, layer) // unpack into layer location
// 	if err != nil { ... }
//
// When the above completes, we should have a filesystem the represents the
// contents of the layer. Careful implementations should verify that digest
// matches the expected DiffID. When completed, we unmount the mounts:
//
//	unmount(mounts) // optional, for now
//
// Now that we've verified and unpacked our layer, we create a location to
// commit the actual diff. For this example, we are just going to use the layer
// digest, but in practice, this will probably be the ChainID:
//
// 	diffPath := filepath.Join("/layers", digest) // name location for the uncompressed layer digest
//	if err := lm.Commit(diffPath, tmpLocation); err != nil { ... }
//
// Now, we have a layer in the LayerManipulator that can be accessed with the
// opaque diffPath provided during commit.
//
// Importing the Next Layer
//
// Making a layer depend on the above is identical to the process described
// above except that the parent is provided as diffPath when calling
// LayerManipulator.Prepare:
//
// 	mounts, err := lm.Prepare(tmpLocation, parentDiffPath)
//
// The diff will be captured at tmpLocation, as the layer is applied.
//
// Running a Container
//
// To run a container, we simply provide LayerManipulator.Prepare the diffPath
// of the image we want to start the container from. After mounting, the
// prepared path can be used directly as the container's filesystem:
//
// 	mounts, err := lm.Prepare(containerRootFS, imageDiffPath)
//
// The returned mounts can then be passed directly to the container runtime. If
// one would like to create a new image from the filesystem,
// LayerManipulator.Commit is called:
//
// 	if err := lm.Commit(newImageDiff, containerRootFS); err != nil { ... }
//
// Alternatively, for most container runs, LayerManipulator.Rollback will be
// called to signal LayerManipulator to abandon the changes.
//
// TODO(stevvooe): Consider an alternate API that provides an active object to
// represent the lifecycle:
//
// 	work, err := lm.Prepare(dst, parent)
//  mountAll(work.Mounts())
// 	work.Commit() || work.Rollback()
type LayerManipulator struct{}

// Prepare returns a set of mounts such that dst can be used as a location for
// reading and writing data. If parent is provided, the dst will be setup to
// capture changes between dst and parent. The "default" parent, "", is an
// empty directory.
//
// If the caller intends to write data to dst, they should perform all mounts
// provided before doing so. The location defined by dst should be used as the
// working directory for any associated activity, such as running a container
// or importing a layer.
//
// Once the writes have completed, LayerManipulator.Commit or
// LayerManipulator.Rollback should be called on dst.
func (lm *LayerManipulator) Prepare(dst, parent string) ([]Mount, error) {
	return nil, errNotImplemented
}

// Commit captures the changes between dst and its parent into the path
// provided by diff. The path diff can then be used with the layer
// manipulator's other methods to access the diff content.
//
// The contents of diff are opaque to the caller and may be specific to the
// implementation of the layer backend.
func (lm *LayerManipulator) Commit(diff, dst string) error {
	return errNotImplemented
}

// Rollback can be called after prepare if the caller would like to abandon the
// changeset.
func (lm *LayerManipulator) Rollback(dst string) error {
	return errNotImplemented
}

func (lm *LayerManipulator) Parent(diff string) string {
	return ""
}

type ChangeKind int

const (
	ChangeKindAdd = iota
	ChangeKindModify
	ChangeKindDelete
)

func (k ChangeKind) String() string {
	switch k {
	case ChangeKindAdd:
		return "add"
	case ChangeKindModify:
		return "modify"
	case ChangeKindDelete:
		return "delete"
	default:
		return ""
	}
}

// Change represents single change between a diff and its parent.
//
// TODO(stevvooe): There are some cool tricks we can do with this type. If we
// provide the path to the resource from both the diff and its parent, for
// example, we can have the differ actually decide the granularity represented
// in the final changeset.
type Change struct {
	Kind ChangeKind
	Path string
}

// Changes returns the list of changes from the diff's parent.
func (lm *LayerManipulator) Changes(diff string) ([]Change, error) {
	return nil, errNotImplemented
}
