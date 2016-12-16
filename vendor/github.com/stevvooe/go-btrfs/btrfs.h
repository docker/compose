#include <stddef.h>
#include <linux/magic.h>
#include <btrfs/ioctl.h>
#include <btrfs/ctree.h>

// unfortunately, we need to define "alignment safe" C structs to populate for
// packed structs that aren't handled by cgo. Fields will be added here, as
// needed.

struct gosafe_btrfs_root_item {
	u8 uuid[BTRFS_UUID_SIZE];
	u8 parent_uuid[BTRFS_UUID_SIZE];
	u8 received_uuid[BTRFS_UUID_SIZE];

	u64 gen;
	u64 ogen;
	u64 flags;
};

void unpack_root_item(struct gosafe_btrfs_root_item* dst, struct btrfs_root_item* src);
/* void unpack_root_ref(struct gosafe_btrfs_root_ref* dst, struct btrfs_root_ref* src); */
