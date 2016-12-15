# Snapshots

Docker containers, from the beginning, have long been built on a snapshotting
methodology known as _layers_. _Layers_ provide the ability to fork a
filesystem, make changes then save the changeset back to a new layer.

Historically, these have been tightly integrated into the Docker daemon as a
component called the `graphdriver`. The `graphdriver` allows one to run the
docker daemon on several different operating systems while still maintaining
roughly similar snapshot semantics for committing and distributing changes to
images.

The `graphdriver` is deeply integrated with the import and export of images,
including managing layer relationships and container runtime filesystems. The
behavior of the `graphdriver` informs the transport of image formats.

In this document, we propose a more flexible model for managing layers. It
focuses on providing an API for the base snapshotting functionality without
coupling so tightly to the structure of images and their identification. The
minimal API simplifies behavior without sacrificing power. This makes the
surface area for driver implementations smaller, ensuring that behavior is more
consistent between implementations.

These differ from the concept of the graphdriver in that the LayerManipulator
has no knowledge of images or containers. Users simply prepare and commit
directories. We also avoid the integration between graph drivers and the tar
format used to represent the changesets.

The best aspect is that we can get to this model by refactoring the existing
graphdrivers, minimizing the need to new code and sprawling tests.

## Scope

In the past, the `graphdriver` component has provided quite a lot of
funcionality in Docker. This includes serialization, hashing, unpacking,
packing, mounting.

This _snapshot manager_ will only provide mount-oriented snapshot
access with minimal metadata. Serialization, hashing, unpacking, packing and
mounting are not included in this design, opting for common implementations
between graphdrivers, rather than specialized ones. This is less of a problem
for performance, since direct access to changesets is provided in the
interface.

## Architecture

The _Snapshot Manager_ provides an API for allocating, snapshotting and mounting
abstract, layer-based filesytems. The model works by building up sets of
directories with parent-child relationships, known as _Snapshots_.

Every snapshot is represented by an opaque `diff` directory, which acts as a
handle to the snapshot. It may contain driver specifc data, including changeset
data, parent information and arbitrary metadata.

The `diff` directory for a _snapshot_ is created with a transactional
operation. Each _snapshot_ may have one parent snapshot. When one starts a
transaction on an existing snapshot, the result may only be used as a parent
_after_ being committed.  The empty string `diff` directory is a handle to the
empty snapshot, which is the ancestor of all snapshots.

The `target` directory represents the active snapshot location. The driver may
maintain internal metadata associated with the `target` but the contents is
generally manipulated by the client.

### Operations

The manifestation of _snapshots_ is facilitated by the _mount_ object and
user-defined directories used for opaque data storage. When creating a new
snapshot, the caller provides a directory where they would like the _snapshot_
to be mounted, called the _target_. This operation returns a list of mounts
that, if mounted, will have the fully prepared snapshot at the requested path.
We call this the _prepare_ operation.

Once a path is _prepared_ and mounted, the caller may write new data to the
snapshot. Depending on application, a user may want to capture these changes or
not.

If the user wants to keep the changes, the _commit_ operation is employed.  The
_commit_ operation takes the `target` directory, which represents an open
transaction, and a `diff` directory. A successful result will end up with the
difference between the parent and snapshot in the `diff` directory, which
should be treated as opaque by the caller. This new `diff` directory can then
be used as the `parent` in calls to future _prepare_ operations.

If the user wants to discard the changes, the _rollback_ operation will release
any resources associated with the snapshot. While rollback may a rare operation
in other transactional systems, this is a common operation for containers.
After removal, most containers will have _rollback_ called.

For both _rollback_ and _commit_ the mounts provided by _prepare_ should be
unmounted before calling these methods.

### Graph metadata

As snapshots are imported into the container system, a "graph" of snapshots and
their parents will form. Queries over this graph must be a supported operation.
Subsequently, each snapshot ends up representing 

### Path Management

No path layout for snapshot locations is imposed on the caller. The paths used
by the snapshot drivers are largely under control of the caller. This provides
the most flexibility in using the snapshot system but requires discipline when
deciding which paths to use and which ones to avoid.

We may provide a helper component to manage `diff` path layout when working
with OCI and docker images.

## How snapshots work

To bring the terminology of _snapshots_, we are going to demonstrate the use of
the _snapshot manager_ from perspective of importing layers. We'll use a Go API
to represent the process.

### Importing a Layer

To import a layer, we simply have the _Snapshot Manager_ provide a list of
mounts to be applied such that our dst will capture a changeset. We start
out by getting a path to the layer tar file and creating a temp location to
unpack it to:

	layerPath, tmpLocation := getLayerPath(), mkTmpDir() // just a path to layer tar file.

Per the terminology above, `tmpLocation` is known as the `target`. `layerPath`
is simply a tar file, representing a changset. We start by using
`SnapshotManager` to prepare the temporary location as a snapshot point:

	lm := SnapshotManager()
	mounts, err := lm.Prepare(tmpLocation, "")
	if err != nil { ... }

Note that we provide "" as the `parent`, since we are applying the diff to an
empty directory. We get back a list of mounts from `SnapshotManager.Prepare`.
Before proceeding, we perform all these mounts:

	if err := MountAll(mounts); err != nil { ... }

Once the mounts are performed, our temporary location is ready to capture
a diff. In practice, this works similar to a filesystem transaction. The
next step is to unpack the layer. We have a special function, `unpackLayer`
that applies the contents of the layer to target location and calculates the
DiffID of the unpacked layer (this is a requirement for docker
implementation):

	digest, err := unpackLayer(tmpLocation, layer) // unpack into layer location
	if err != nil { ... }

When the above completes, we should have a filesystem the represents the
contents of the layer. Careful implementations should verify that digest
matches the expected DiffID. When completed, we unmount the mounts:

	unmount(mounts) // optional, for now

Now that we've verified and unpacked our layer, we create a location to commit
the actual diff. For this example, we are just going to use the layer `digest`,
but in practice, this will probably be the `ChainID`:

	diffPath := filepath.Join("/layers", digest) // name location for the uncompressed layer digest
	if err := lm.Commit(diffPath, tmpLocation); err != nil { ... }

The new layer has been imported as a _snapshot_ into the `SnapshotManager`
under the name `diffPath`. `diffPath`, which is a user opaque directory
location, can then be used as a parent in later snapshots.

### Importing the Next Layer

Making a layer depend on the above is identical to the process described
above except that the parent is provided as diffPath when calling
`Snapshot.Prepare`:

	mounts, err := lm.Prepare(tmpLocation, parentDiffPath)

Because have a provided a `parent`, the resulting `tmpLocation`, after
mounting, will have the changes from above. Any new changes will be isolated to
the snapshot `target`.

We run the same unpacking process and commit as above to get the new `diff`.

### Running a Container

To run a container, we simply provide `SnapshotManager.Prepare` the `diff` of
the image we want to start the container from. After mounting, the prepared
path can be used directly as the container's filesystem:

	mounts, err := lm.Prepare(containerRootFS, imageDiffPath)

The returned mounts can then be passed directly to the container runtime. If
one would like to create a new image from the filesystem,
SnapshotManipulator.Commit is called:

	if err := lm.Commit(newImageDiff, containerRootFS); err != nil { ... }

Alternatively, for most container runs, Snapshot.Rollback will be
called to signal `SnapshotManager` to abandon the changes.
