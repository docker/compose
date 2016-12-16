package btrfs

// Info describes metadata about a btrfs subvolume.
type Info struct {
	ID         uint64 // subvolume id
	ParentID   uint64 // aka ref_tree
	TopLevelID uint64 // not actually clear what this is, not set for now.
	Offset     uint64 // key offset for root
	DirID      uint64

	Generation         uint64
	OriginalGeneration uint64

	UUID         string
	ParentUUID   string
	ReceivedUUID string

	Name string
	Path string // absolute path of subvolume
	Root string // path of root mount point

	Readonly bool // true if the snaps hot is readonly, extracted from flags
}

type infosByID []Info

func (b infosByID) Len() int           { return len(b) }
func (b infosByID) Less(i, j int) bool { return b[i].ID < b[j].ID }
func (b infosByID) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
